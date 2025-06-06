/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import DefaultWebSocket from 'ws';
import { OrraLogger } from './logger.js';
import { TaskAbortedError } from './errors.js'

import { promises as fs } from 'fs';
import path from 'path';

const DEFAULT_SERVICE_KEY_DIR = '.orra-data'
const DEFAULT_SERVICE_KEY_FILE = 'orra-service-key.json'

class OrraSDK {
	#apiUrl;
	#apiKey;
	#ws;
	#taskHandler;
	#revertHandler;
	#revertible = false;
	#revertTTL = 24 * 60 * 60 * 1000; // 24 hours
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
	#userInitiatedClose = false;
	#cacheCleanupIntervalId = null;
	
	constructor({ serviceName, connection, persistence, WebSocketImpl }) {
		this.WebSocket = WebSocketImpl || DefaultWebSocket;
		this.#apiUrl = connection.orraUrl;
		this.#apiKey = connection.orraKey;
		this.#ws = null;
		this.#taskHandler = null;
		this.name = serviceName
		this.serviceId = null;
		this.version = 0;
		this.persistenceOpts = {
			method: 'file', // 'file' or 'custom'
			filePath: path.join(process.cwd(), DEFAULT_SERVICE_KEY_DIR, DEFAULT_SERVICE_KEY_FILE),
			customSave: null,
			customLoad: null,
			...persistence
		};
		this.#startProcessedTasksCacheCleanup()
		this.logger = new OrraLogger({});
	}
	
	async saveServiceKey() {
		if (this.persistenceOpts.method === 'file') {
			const data = JSON.stringify({ id: this.serviceId });
			const filePath = this.persistenceOpts.filePath
			const directoryPath = extractDirectoryFromFilePath(filePath);
			await createDirectoryIfNotExists(directoryPath, this.logger);
			
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
				this.serviceId = parsed.id;
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
		if (!name) {
			throw new Error('Cannot register service: name is required');
		}
		return this.#registerServiceOrAgent(name, "service", opts);
	}
	
	async registerAgent(name, opts = {
		description: undefined,
		schema: undefined,
	}) {
		if (!name) {
			throw new Error('Cannot register agent: name is required');
		}
		return this.#registerServiceOrAgent(name, "agent", opts);
	}
	
	pushUpdate(taskId, executionId, idempotencyKey, updateData) {
		if (!taskId || !executionId || !idempotencyKey) {
			this.logger.error('Cannot push update: taskId, executionId and idempotencyKey are all required', {
				taskId,
				executionId,
				idempotencyKey
			});
			return Promise.reject(new Error('Both taskId, executionId and idempotencyKey are required for pushing updates'));
		}
		
		if (!this.serviceId) {
			this.logger.error('Cannot push update: service not registered', {
				taskId,
				executionId,
				idempotencyKey
			});
			return Promise.reject(new Error('Service must be registered before pushing updates'));
		}
		
		if (this.#userInitiatedClose) {
			this.logger.error('Cannot push update: SDK is shutting down', {
				taskId,
				executionId,
				idempotencyKey
			});
			return Promise.reject(new Error('SDK is shutting down'));
		}
		
		return new Promise((resolve, reject) => {
			try {
				// Wrap the update data in the expected format
				const payload = {
					task: updateData
				};
				
				this.#sendInterimTaskResult(taskId, executionId, this.serviceId, idempotencyKey, payload);
				resolve();
			} catch (error) {
				this.logger.error('Failed to push update', {
					taskId,
					executionId,
					idempotencyKey,
					error: error.message
				});
				reject(error);
			}
		});
	}
	
	abort(taskId, executionId, idempotencyKey, abortPayload) {
		if (!taskId || !executionId || !idempotencyKey) {
			this.logger.error('Cannot abort: taskId, executionId and idempotencyKey are all required', {
				taskId,
				executionId,
				idempotencyKey
			});
			return Promise.reject(new Error('Both taskId, executionId and idempotencyKey are required for aborting tasks'));
		}
		
		if (!this.serviceId) {
			this.logger.error('Cannot abort: service not registered', {
				taskId,
				executionId,
				idempotencyKey
			});
			return Promise.reject(new Error('Service must be registered for tasks to be aborted'));
		}
		
		if (this.#userInitiatedClose) {
			this.logger.error('Cannot abort: SDK is shutting down', {
				taskId,
				executionId,
				idempotencyKey
			});
			return Promise.reject(new Error('SDK is shutting down'));
		}
		
		return new Promise((resolve, reject) => {
			try {
				// Wrap the update data in the expected format
				const payload = {
					task: abortPayload
				};
				
				this.#sendAbortTaskResult(taskId, executionId, this.serviceId, idempotencyKey, payload);
				resolve(abortPayload);
			} catch (error) {
				this.logger.error('Failed to abort', {
					taskId,
					executionId,
					idempotencyKey,
					error: error.message
				});
				reject(error);
			}
		});
	}
	
	async #registerServiceOrAgent(name, kind, opts = {
		description: undefined,
		revertible: undefined,
		revertTTL: undefined,
		schema: undefined,
	}) {
		if (this.#userInitiatedClose) {
			throw new Error(`Cannot register ${kind} after closing down SDK connections`)
		}
		
		this.#validateSchema(opts, kind);
		
		if (opts.revertible !== undefined) {
			if (typeof opts.revertible !== 'boolean') throw new Error(`${kind} revertible must be boolean (true or false)`);
			this.#revertible = opts.revertible
		}
		
		if (opts.revertTTL !== undefined) {
			if (typeof opts.revertTTL !== 'number') throw new Error(`${kind} revert TTL must be a number of milliseconds`);
			if (opts.revertTTL <= 100) throw new Error(`${kind} If specified, revert TTL must be greater than 100 milliseconds`);
			this.#revertTTL = opts.revertTTL
		}
		
		await this.loadServiceKey(); // Try to load an existing service id
		
		this.logger.debug('Registering service/agent', {
			kind,
			name,
			existingServiceId: this.serviceId,
			revertible: this.#revertible
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
				revertible: this.#revertible,
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
	
	#validateSchema(opts, kind) {
		if (opts?.schema) {
			if (!opts.schema?.input || !opts.schema?.output) {
				throw new Error(`${kind} schema must contain input and output specifications`);
			}
		}
	}
	
	#connect() {
		if (this.#userInitiatedClose) {
			throw new Error(`Cannot request halted as SDK connections closed permanently.`)
		}
		
		const wsUrl = this.#apiUrl.replace('http', 'ws');
		this.#ws = new this.WebSocket(`${wsUrl}/ws?serviceId=${this.serviceId}&apiKey=${this.#apiKey}`);
		
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
				case 'ping':
					this.#handlePing(parsedData);
					break;
				case 'ACK':
					this.#handleAcknowledgment(parsedData);
					break;
				case 'task_request':
					this.#handleTask(parsedData);
					break;
				case 'compensation_request':
					this.#handleRevert(parsedData);
					break;
				default:
					this.logger.warn('Received unknown message type', {
						type: parsedData.type
					});
			}
		};
		
		this.#ws.onclose = (event) => {
			this.#isConnected = false;
			
			if (this.#userInitiatedClose) {
				this.logger.info('WebSocket closed by user', { code: event.code, reason: event.reason });
				return; // Do not reconnect
			}
			
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
	
	#handlePing(data) {
		if (data.serviceId !== this.serviceId) {
			this.logger.trace(`Received PING for unknown serviceId: ${data.serviceId}`);
			return
		}
		this.logger.trace("Received PING");
		this.#sendPong();
		this.logger.trace("Sent PONG");
	}
	
	#sendPong() {
		if (this.#isConnected && this?.#ws?.readyState === this.WebSocket.OPEN) {
			this?.#ws?.send(JSON.stringify({ id: "pong", payload: { type: 'pong', serviceId: this.serviceId } }));
		}
	}
	
	#handleAcknowledgment(data) {
		this.logger.trace("Acknowledging already sent message", { msgId: data.id });
		this.#pendingMessages.delete(data.id);
	}
	
	#handleTask(task) {
		const { id: taskId, executionId, idempotencyKey } = task;
		
		task.pushUpdate = (updateData) => {
			return this.pushUpdate(taskId, executionId, idempotencyKey, updateData);
		};
		
		task.abort = (abortPayload) => {
			return this.abort(taskId, executionId, idempotencyKey, abortPayload).then(abortPayload => {
				// Throw the error here to terminate the handler
				throw new TaskAbortedError(abortPayload);
			});
		}
		
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
		if (processedResult && processedResult?.status === 'aborted') {
			return;
		}
		
		if (processedResult) {
			this.logger.debug('Cache hit found', {
				taskId,
				idempotencyKey,
				resultAge: Date.now() - processedResult.timestamp
			});
			this.#sendTaskResult(
				taskId,
				executionId,
				this.serviceId,
				idempotencyKey,
				processedResult.result
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
			.then((taskResult) => {
				const result = {
					task: taskResult,
					compensation: this.#revertible ? {
						data: {
							originalTask: task,
							taskResult: taskResult
						},
						ttl: this.#revertTTL,
					} : null
				}
				
				const processingTime = Date.now() - startTime;
				this.logger.trace('Task processing completed', {
					taskId,
					executionId,
					idempotencyKey,
					processingTimeMs: processingTime,
					resultSize: JSON.stringify(result).length
				});
				
				this.#processedTasksCache.set(idempotencyKey, {
					result: result,
					timestamp: Date.now()
				});
				
				this.#inProgressTasks.delete(idempotencyKey);
				this.#sendTaskResult(taskId, executionId, this.serviceId, idempotencyKey, result);
			})
			.catch((error) => {
				if (error instanceof TaskAbortedError) {
					// Handle task abortion - this is the single place where it's handled
					this.logger.debug('Task execution aborted', {
						taskId, executionId, idempotencyKey
					});
					
					// Clean up in-progress task
					this.#inProgressTasks.delete(idempotencyKey);
					
					// Add to processed tasks cache with abort status
					this.#processedTasksCache.set(idempotencyKey, {
						result: { task: error.abortPayload, status: 'aborted' },
						timestamp: Date.now()
					});
				} else {
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
					this.#inProgressTasks.delete(idempotencyKey);
					this.#sendTaskResult(taskId, executionId, this.serviceId, idempotencyKey, null, error.message);
				}
			});
	}
	
	#handleRevert(task) {
		const { id: taskId, executionId, idempotencyKey, input, compensationContext = null } = task;
		
		this.logger.trace('Revert task handling initiated', {
			taskId,
			executionId,
			idempotencyKey,
			handlerPresent: !!this.#taskHandler,
			timestamp: new Date().toISOString()
		});
		
		if (!this.#revertHandler) {
			this.logger.warn('Received revert task but no revert handler is set', {
				taskId,
				executionId
			});
			return;
		}
		
		this.logger.trace('Checking task cache for revert task', {
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
				resultAge: Date.now() - processedResult.timestamp
			});
			this.#sendTaskResult(
				taskId,
				executionId,
				this.serviceId,
				idempotencyKey,
				processedResult.result
			);
			return;
		}
		
		this.logger.trace('Checking in-progress reverts', {
			taskId,
			idempotencyKey,
			inProgressCount: this.#inProgressTasks.size,
			checkTimestamp: new Date().toISOString()
		});
		
		if (this.#inProgressTasks.has(idempotencyKey)) {
			this.logger.debug('Revert already in progress', {
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
		this.logger.trace('Starting new revert processing', {
			taskId,
			executionId,
			idempotencyKey,
			startTime: new Date(startTime).toISOString()
		});
		
		this.#inProgressTasks.set(idempotencyKey, { startTime });
		
		Promise.resolve()
			.then(() => this.#revertHandler(input.originalTask, input.taskResult, compensationContext))
			.then((rawResult) => {
				const result = processRevertResult(rawResult, this.logger, taskId, executionId);
				
				const processingTime = Date.now() - startTime;
				this.logger.trace('Revert processing completed', {
					taskId,
					executionId,
					idempotencyKey,
					processingTimeMs: processingTime,
					resultSize: JSON.stringify(result).length
				});
				
				this.#processedTasksCache.set(idempotencyKey, {
					result: result,
					timestamp: Date.now()
				});
				
				this.#inProgressTasks.delete(idempotencyKey);
				this.#sendTaskResult(taskId, executionId, this.serviceId, idempotencyKey, result);
			})
			.catch((error) => {
				const processingTime = Date.now() - startTime;
				this.logger.trace('Revert processing failed', {
					taskId,
					executionId,
					idempotencyKey,
					processingTimeMs: processingTime,
					errorType: error.constructor.name,
					errorMessage: error.message,
					stackTrace: error.stack
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
	
	#sendInterimTaskResult(taskId, executionId, serviceId, idempotencyKey, result) {
		const message = {
			type: 'task_interim_result',
			taskId,
			executionId,
			serviceId,
			idempotencyKey,
			result
		};
		this.#sendMessage(message);
	}
	
	#sendAbortTaskResult(taskId, executionId, serviceId, idempotencyKey, result) {
		const message = {
			type: 'task_abort_result',
			taskId,
			executionId,
			serviceId,
			idempotencyKey,
			result
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
		
		if (this.#isConnected && this?.#ws?.readyState === this.WebSocket.OPEN) {
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
		while (this.#messageQueue.length > 0 && this.#isConnected && this?.#ws?.readyState === this.WebSocket.OPEN) {
			const message = this.#messageQueue.shift();
			this.#ws.send(JSON.stringify(message));
			this.logger.debug('Sent queued message', {
				message,
			});
		}
	}
	
	#startProcessedTasksCacheCleanup() {
		this.#cacheCleanupIntervalId = setInterval(() => {
			const now = Date.now();
			let processedTasksRemoved = 0;
			let inProgressTasksRemoved = 0;
			
			for (const [ key, data ] of this.#processedTasksCache.entries()) {
				if (now - data.timestamp > this.#maxProcessedTasksAge) {
					this.#processedTasksCache.delete(key);
					processedTasksRemoved++;
				}
			}
			
			for (const [ key, data ] of this.#inProgressTasks.entries()) {
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
	
	isRevertible() {
		return this.#revertible;
	}
	
	getRevertTTL() {
		if (!this.isRevertible()) return undefined;
		return this.#revertTTL;
	}
	
	revertHandler(handler) {
		if (typeof handler !== 'function') {
			throw new Error('Revert handler must be a function');
		}
		this.#revertHandler = handler;
		this.logger.debug('Revert handler registered');
	}
	
	startHandler(handler) {
		if (typeof handler !== 'function') {
			throw new Error('Start handler must be a function');
		}
		if (this.#revertible && !this.#revertHandler) {
			throw new Error('onRevert handler is missing');
		}
		this.#taskHandler = handler;
	}
	
	shutdown() {
		this.logger.info('User initiated WebSocket close');
		// Set flag indicating that the closure was initiated by the user
		this.#userInitiatedClose = true;
		
		// Close WebSocket cleanly with normal closure code (1000)
		if (this?.#ws?.readyState === this.WebSocket.OPEN || this?.#ws?.readyState === this.WebSocket.CONNECTING) {
			this?.#ws?.close(1000, 'Normal Closure');
		}
		
		if (this.#cacheCleanupIntervalId !== null) {
			clearInterval(this.#cacheCleanupIntervalId);
			this.logger.trace('Cleared cache cleanup interval after user initiated close');
			this.#cacheCleanupIntervalId = null;
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

async function createDirectoryIfNotExists(directoryPath, logger) {
	let exists = false
	try {
		exists = await directoryExists(directoryPath)
		if (exists) return
		
		try {
			await fs.mkdir(directoryPath, { recursive: true });
			
			logger.trace('Directory created successfully', { directoryPath });
		} catch (mkdirError) {
			logger.error('Error creating directory', {
				error: mkdirError.message,
				directoryPath
			});
		}
	} catch (e) {
		logger.error('Error creating directory', {
			error: e.message,
			directoryPath
		});
	}
}

function validatePartialResult(raw) {
	if (!raw?.partial) {
		return { valid: false, error: 'Missing partial field' };
	}
	
	const { completed, remaining } = raw.partial;
	
	if (!Array.isArray(completed)) {
		return { valid: false, error: 'completed must be an array' };
	}
	
	if (!Array.isArray(remaining)) {
		return { valid: false, error: 'remaining must be an array' };
	}
	
	return { valid: true, value: raw };
}

function processRevertResult(v, logger, taskId, executionId) {
	if (v === undefined || typeof v !== 'object') {
		return {
			status: 'completed',
		};
	}
	
	const validation = validatePartialResult(v);
	if (validation.valid) {
		return {
			status: 'partial',
			...validation.value
		};
	}
	
	logger.warn('Revert result reporting failed', {
		taskId,
		executionId,
		error: validation.error
	});
	
	return {
		status: 'completed',
	};
}

const validateName = (name, type) => {
	if (!name || typeof name !== 'string') {
		throw new Error(`${type} name must be a non-empty string`);
	}
	
	if (name.length < 3 || name.length > 63) {
		throw new Error(`${type} name must be between 3 and 63 characters`);
	}
	
	const validNamePattern = /^[a-z0-9][a-z0-9.-]*[a-z0-9]$/;
	if (!validNamePattern.test(name)) {
		throw new Error(`${type} name must contain only lowercase alphanumeric characters, dots, and hyphens, and must start and end with an alphanumeric character`);
	}
};

const initOrraEntity = (type) => ({
	                                  name,
	                                  orraUrl,
	                                  orraKey,
	                                  persistenceOpts = {},
	                                  WebSocketImpl,
                                  }) => {
	validateName(name, type);
	
	if (!name) {
		throw new Error(`Cannot create ${type}: ensure a valid name has been provided.`);
	}
	
	if (!orraUrl || !orraKey) {
		throw new Error(`Cannot create ${type}: ensure both a valid Orra URL and Orra API Key have been provided.`);
	}
	
	const sdk = new OrraSDK({
		serviceName: name,
		connection: {
			orraUrl,
			orraKey
		},
		persistence: {
			filePath: path.join(process.cwd(), DEFAULT_SERVICE_KEY_DIR, `${name}-${DEFAULT_SERVICE_KEY_FILE}`),
			...persistenceOpts
		},
		WebSocketImpl
	});
	
	const registerMethod = type === 'agent' ? sdk.registerAgent : sdk.registerService;
	
	return {
		register: async (opts) => {
			return await registerMethod.call(sdk, sdk.name, opts);
		},
		onRevert: sdk.revertHandler.bind(sdk),
		start: sdk.startHandler.bind(sdk),
		shutdown: sdk.shutdown.bind(sdk),
		info: {
			get id() {
				return sdk.serviceId;
			},
			get version() {
				return sdk.version;
			},
			get revertible() {
				return sdk.isRevertible();
			},
			get revertTTL() {
				return sdk.getRevertTTL();
			}
		}
	};
};

export const initService = initOrraEntity('service');
export const initAgent = initOrraEntity('agent');
