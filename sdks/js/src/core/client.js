/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import WebSocket from 'ws';
import { OrraLogger } from './logger.js';

import { promises as fs } from 'fs';
import path from 'path';

const DEFAULT_SERVICE_KEY_FILE= 'orra-service-key.json'

class OrraSDK {
	#apiUrl;
	#apiKey;
	#ws;
	#taskHandler;
	serviceId;
	version;
	persistenceOpts;
	#reconnectAttempts = 0;
	#maxReconnectAttempts = 10;
	#reconnectInterval = 1000; // 1 seconds
	#maxReconnectInterval = 30000 // Max 30 seconds
	#messageQueue = [];
	#isConnected = false;
	#messageId = 0;
	#pendingMessages = new Map();
	#processedTasksCache = new Map();
	#inProgressTasks = new Map();
	#maxProcessedTasksAge = 24 * 60 * 60 * 1000; // 24 hours
	#maxInProgressAge = 30 * 60 * 1000; // 30 minutes
	
	constructor(apiUrl, apiKey, persistenceOpts={}) {
		this.#apiUrl = apiUrl;
		this.#apiKey = apiKey;
		this.#ws = null;
		this.#taskHandler = null;
		this.serviceId = null;
		this.version = 0;
		this.persistenceOpts = {
			method: 'file', // 'file' or 'custom'
			filePath: path.join(process.cwd(), DEFAULT_SERVICE_KEY_FILE),
			customSave: null,
			customLoad: null,
			...persistenceOpts
		};
		this.#startProcessedTasksCacheCleanup()
		this.logger = new OrraLogger({});
	}
	
	async saveServiceKey() {
		if (this.persistenceOpts.method === 'file') {
			const data = JSON.stringify({ serviceId: this.serviceId });
			const filePath = this.persistenceOpts.filePath
			const directoryPath = extractDirectoryFromFilePath(filePath);
			await createDirectoryIfNotExists(directoryPath);
			
			await fs.writeFile(this.persistenceOpts.filePath, data, 'utf8');
		} else if (this.persistenceOpts.method === 'custom' && typeof this.persistenceOpts.customSave === 'function') {
			await this.persistenceOpts.customSave(this.serviceId);
		}
	}
	
	async loadServiceKey() {
		try {
			if (this.persistenceOpts.method === 'file') {
				
				const filePath = this.persistenceOpts.filePath;
				const directoryPath = extractDirectoryFromFilePath(filePath);
				const exists = await directoryExists(directoryPath);
				if (!exists) return;
				
				const data = await fs.readFile(filePath, 'utf8');
				const parsed = JSON.parse(data);
				this.serviceId = parsed.serviceId;
			} else if (this.persistenceOpts.method === 'custom' && typeof this.persistenceOpts.customLoad === 'function') {
				this.serviceId = await this.persistenceOpts.customLoad();
			}
		} catch (error) {
			// If loading fails, we'll keep the serviceId as null and get a new one upon registration
		}
	}
	
	async registerService(name, opts = {
		description: undefined,
		schema: undefined,
	}) {
		return this.#registerServiceOrAgent(name, "service", opts);
	}
	
	async registerAgent(name, opts = {
		description: undefined,
		schema: undefined,
	}) {
		return this.#registerServiceOrAgent(name, "agent", opts);
	}
	
	async #registerServiceOrAgent(name, kind, opts = {
		description: undefined,
		schema: undefined,
	}) {
		await this.loadServiceKey(); // Try to load an existing service id
		
		this.logger.debug('Registering service/agent', {
			kind,
			name,
			existingServiceId: this.serviceId
		});
		
		const response = await fetch(`${this.#apiUrl}/register/${kind}`, {
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
				'Authorization': `Bearer ${this.#apiKey}`
			},
			body: JSON.stringify({
				id: this.serviceId,
				name: name,
				description: opts?.description,
				schema: opts?.schema,
				version: this.version,
			}),
		});
		
		if (!response.ok) {
			const resText = await response.text()
			const error = `Failed to register ${kind} because of ${response.statusText}: ${resText}`
			this.logger.error(error, {
				statusCode: response.status,
				responseText: resText,
				kind,
				name
			});
			throw new Error(error);
		}
		
		const data = await response.json();
		this.serviceId = data.id;
		if (!this.serviceId) {
			const error = `${kind} ID was not received after registration`
			this.logger.error(error, { response: data });
			throw new Error(error);
		}
		this.version = data.version;
		
		this.logger.reconfigure({
			serviceId: this.serviceId,
			serviceVersion: this.version
		});
		
		this.logger.info(`Successfully registered ${kind}`, {
			name,
		});
		
		await this.saveServiceKey(); // Save the new or updated key
		this.#connect();
		return this;
	}
	
	#connect() {
		const wsUrl = this.#apiUrl.replace('http', 'ws');
		this.#ws = new WebSocket(`${wsUrl}/ws?serviceId=${this.serviceId}&apiKey=${this.#apiKey}`);
		
		this.logger.debug('Initiating WebSocket connection');
		
		this.#ws.onopen = () => {
			this.#isConnected = true;
			this.#reconnectAttempts = 0;
			this.#reconnectInterval = 1000;
			
			this.logger.info('WebSocket connection established');
			
			this.#sendQueuedMessages();
		};
		
		this.#ws.onmessage = (event) => {
			const data = event.data;
			
			if (data === 'ping') {
				this.logger.trace('Received ping');
				this.#handlePing();
				return;
			}
			
			let parsedData;
			try {
				parsedData = JSON.parse(data);
				this.logger.trace('Received message', { messageType: parsedData.type });
			} catch (error) {
				this.logger.error('Failed to parse WebSocket message', {
					error: error.message,
					data
				});
				return;
			}
			
			switch (parsedData.type) {
				case 'ACK':
					this.#handleAcknowledgment(parsedData);
					break;
				case 'task_request':
					this.#handleTask(parsedData);
					break;
				default:
					this.logger.warn('Received unknown message type', {
						type: parsedData.type
					});
			}
		};
		
		this.#ws.onclose = (event) => {
			this.#isConnected = false;
			for (const message of this.#pendingMessages.values()) {
				this.#messageQueue.push(message);
			}
			this.#pendingMessages.clear();
			
			const meta = {
				code: event.code,
				reason: event.reason,
				wasClean: event.wasClean
			};
			
			if (event.wasClean) {
				this.logger.info('WebSocket closed cleanly', meta);
			} else {
				this.logger.warn('WebSocket connection died', meta);
			}
			
			this.#reconnect();
		};
		
		this.#ws.onerror = (error) => {
			this.logger.error('WebSocket error', {
				error: error.message
			});
		};
	}
	
	#handlePing() {
		this.logger.trace("Received PING");
		this.#sendPong();
		this.logger.trace("Sent PONG");
	}
	
	#sendPong() {
		if (this.#isConnected && this.#ws.readyState === WebSocket.OPEN) {
			this.#ws.send(JSON.stringify({ id: "pong", payload: { type: 'pong', serviceId: this.serviceId } }));
		}
	}
	
	#handleAcknowledgment(data) {
		this.logger.trace("Acknowledging already sent message", { msgId: data.id });
		this.#pendingMessages.delete(data.id);
	}
	
	#handleTask(task) {
		const { id: taskId, executionId, idempotencyKey } = task;
		
		this.logger.trace('Task handling initiated', {
			taskId,
			executionId,
			idempotencyKey,
			handlerPresent: !!this.#taskHandler,
			timestamp: new Date().toISOString()
		});
		
		if (!this.#taskHandler) {
			this.logger.warn('Received task but no task handler is set', {
				taskId,
				executionId
			});
			return;
		}
		
		this.logger.trace('Checking task cache', {
			taskId,
			idempotencyKey,
			cacheSize: this.#processedTasksCache.size,
			checkTimestamp: new Date().toISOString()
		});
		
		const processedResult = this.#processedTasksCache.get(idempotencyKey);
		if (processedResult) {
			this.logger.debug('Cache hit found', {
				taskId,
				idempotencyKey,
				resultAge: Date.now() - processedResult.timestamp,
				hasError: !!processedResult.error
			});
			this.#sendTaskResult(
				taskId,
				executionId,
				this.serviceId,
				idempotencyKey,
				processedResult.result,
				processedResult.error
			);
			return;
		}
		
		this.logger.trace('Checking in-progress tasks', {
			taskId,
			idempotencyKey,
			inProgressCount: this.#inProgressTasks.size,
			checkTimestamp: new Date().toISOString()
		});
		
		if (this.#inProgressTasks.has(idempotencyKey)) {
			this.logger.debug('Task already in progress', {
				taskId,
				idempotencyKey,
				startTime: this.#inProgressTasks.get(idempotencyKey).startTime
			});
			
			this.#sendTaskStatus(
				taskId,
				executionId,
				this.serviceId,
				idempotencyKey,
				'in_progress'
			);
			return;
		}
		
		const startTime = Date.now();
		this.logger.trace('Starting new task processing', {
			taskId,
			executionId,
			idempotencyKey,
			startTime: new Date(startTime).toISOString()
		});
		
		this.#inProgressTasks.set(idempotencyKey, { startTime });
		
		Promise.resolve(this.#taskHandler(task))
			.then((result) => {
				
				const processingTime = Date.now() - startTime;
				this.logger.trace('Task processing completed', {
					taskId,
					executionId,
					idempotencyKey,
					processingTimeMs: processingTime,
					resultSize: JSON.stringify(result).length
				});
				
				this.#processedTasksCache.set(idempotencyKey, {
					result,
					error: null,
					timestamp: Date.now()
				});
				
				this.#inProgressTasks.delete(idempotencyKey);
				this.#sendTaskResult(taskId, executionId, this.serviceId, idempotencyKey, result);
			})
			.catch((error) => {
				const processingTime = Date.now() - startTime;
				this.logger.trace('Task processing failed', {
					taskId,
					executionId,
					idempotencyKey,
					processingTimeMs: processingTime,
					errorType: error.constructor.name,
					errorMessage: error.message,
					stackTrace: error.stack
				});
				
				this.#processedTasksCache.set(idempotencyKey, {
					result: null,
					error: error.message,
					timestamp: Date.now()
				});
				this.#inProgressTasks.delete(idempotencyKey);
				this.#sendTaskResult(taskId, executionId, this.serviceId, idempotencyKey, null, error.message);
			});
	}
	
	
	#reconnect() {
		if (this.#reconnectAttempts >= this.#maxReconnectAttempts) {
			this.logger.error('Max reconnection attempts reached', {
				attempts: this.#reconnectAttempts,
				maxAttempts: this.#maxReconnectAttempts
			});
			return;
		}
		
		this.#reconnectAttempts++;
		const delay = Math.min(this.#reconnectInterval * Math.pow(2, this.#reconnectAttempts), this.#maxReconnectInterval);
		
		this.logger.info('Scheduling reconnection attempt', {
			attempt: this.#reconnectAttempts,
			delayMs: delay
		});
		
		setTimeout(() => {
			this.logger.debug('Attempting to reconnect', {
				attempt: this.#reconnectAttempts
			});
			this.#connect();
		}, delay);
	}
	
	#sendTaskStatus(taskId, executionId, serviceId, idempotencyKey, status) {
		const message = {
			type: 'task_status',
			taskId,
			executionId,
			serviceId,
			idempotencyKey,
			status,
		};
		this.#sendMessage(message);
	}
	
	#sendTaskResult(taskId, executionId, serviceId, idempotencyKey, result, error = null) {
		const message = {
			type: 'task_result',
			taskId,
			executionId,
			serviceId,
			idempotencyKey,
			result,
			error
		};
		this.#sendMessage(message);
	}
	
	#sendMessage(message) {
		this.#messageId++
		const id = `message_${this.#messageId}_${message.executionId}`;
		const wrappedMessage = { id, payload: message };
		
		this.logger.trace('Preparing to send message', {
			messageId: id,
			type: message.type
		});
		
		if (this.#isConnected && this.#ws.readyState === WebSocket.OPEN) {
			try {
				this.#ws.send(JSON.stringify(wrappedMessage));
				this.logger.debug('Message sent successfully', {
					messageId: id,
					type: message.type
				});
				this.#pendingMessages.set(id, message);
				// Set a timeout to move message back to queue if no ACK received
				setTimeout(() => this.#handleMessageTimeout(id), 5000);
				
			} catch (e) {
				this.logger.error('Failed to send message, queueing message', {
					messageId: id,
					error: e.message,
					type: message.type
				});
				this.#messageQueue.push(message);
			}
		} else {
			this.logger.debug('WebSocket not ready, queueing message', {
				messageId: id,
				type: message.type,
				queueLength: this.#messageQueue.length + 1
			});
			this.#messageQueue.push(message);
		}
	}
	
	#handleMessageTimeout(id) {
		if (this.#pendingMessages.has(id)) {
			const message = this.#pendingMessages.get(id);
			this.#pendingMessages.delete(id);
			this.#messageQueue.push(message);
		}
	}
	
	#sendQueuedMessages() {
		while (this.#messageQueue.length > 0 && this.#isConnected && this.#ws.readyState === WebSocket.OPEN) {
			const message = this.#messageQueue.shift();
			this.#ws.send(JSON.stringify(message));
			this.logger.debug('Sent queued message', {
				message,
			});
		}
	}
	
	#startProcessedTasksCacheCleanup() {
		setInterval(() => {
			const now = Date.now();
			let processedTasksRemoved = 0;
			let inProgressTasksRemoved = 0;
			
			for (const [key, data] of this.#processedTasksCache.entries()) {
				if (now - data.timestamp > this.#maxProcessedTasksAge) {
					this.#processedTasksCache.delete(key);
					processedTasksRemoved++;
				}
			}
			
			for (const [key, data] of this.#inProgressTasks.entries()) {
				if (now - data.startTime > this.#maxInProgressAge) {
					this.#inProgressTasks.delete(key);
					inProgressTasksRemoved++;
				}
			}
			
			if (processedTasksRemoved > 0 || inProgressTasksRemoved > 0) {
				this.logger.debug('Cache cleanup completed', {
					processedTasksRemoved,
					inProgressTasksRemoved,
					remainingProcessedTasks: this.#processedTasksCache.size,
					remainingInProgressTasks: this.#inProgressTasks.size
				});
			}
		}, 60 * 60 * 1000); // Run cleanup every hour
	}
	
	startHandler(handler) {
		this.#taskHandler = handler;
	}
	
	close() {
		if (this.#ws) {
			this.#ws.close();
		}
	}
}


function extractDirectoryFromFilePath(filePath) {
	return path.dirname(filePath);
}

async function directoryExists(dirPath) {
	try {
		await fs.access(dirPath, fs.constants.F_OK);
		return true;
	} catch (error) {
		if (error.code === 'ENOENT') {
			return false;
		}
		throw error; // Re-throw any other errors
	}
}

async function createDirectoryIfNotExists(directoryPath) {
	let exists = false
	try {
		exists = await directoryExists(directoryPath)
		if (exists) return
		
		try {
			await fs.mkdir(directoryPath, { recursive: true });
			
			this.logger.trace('Directory created successfully', {directoryPath});
		} catch (mkdirError) {
			this.logger.error('Error creating directory', {
				error: mkdirError.message,
				directoryPath
			});
		}
	}catch (e) {
		this.logger.error('Error creating directory', {
			error: e.message,
			directoryPath
		});
	}
}

export const createClient = (opts = {
	orraUrl: undefined,
	orraKey: undefined,
	persistenceOpts: {},
}) => {
	if (!opts?.orraUrl || !opts?.orraKey) {
		throw new Error("Cannot create an SDK client: ensure both a valid Orra URL and Orra API Key have been provided.");
	}
	return new OrraSDK(opts?.orraUrl, opts?.orraKey, opts?.persistenceOpts);
}
