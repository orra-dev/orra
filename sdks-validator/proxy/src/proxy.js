/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import { createServer } from 'http';
import { WebSocketServer } from 'ws';
import { createHttpProxy } from './http-proxy.js';
import { createWebSocketProxy } from './ws-proxy.js';
import { createWebhookHandler } from './webhook-handler.js';
import { ProtocolValidator } from "./validator.js";

export class ProtocolProxy {
	constructor(controlPlaneUrl, sdkContractPath) {
		this.controlPlaneUrl = controlPlaneUrl;
		this.sdkContractPath = sdkContractPath
		this.httpServer = null;
		this.wsProxy = null;
		this.wsServer = null;
		this.webhookResults = new Map();
		this.validator = new ProtocolValidator(sdkContractPath);
	}
	
	async start(port = 8006) {
		this.httpServer = createServer();
		this.wsServer = new WebSocketServer({ server: this.httpServer });
		
		const httpProxy = createHttpProxy(this.controlPlaneUrl, this.sdkContractPath);
		const webhookHandler = createWebhookHandler(this.webhookResults);
		
		// Handle HTTP requests
		this.httpServer.on('request', (req, res) => {
			const url = new URL(req.url, `http://${req.headers.host}`);
			
			if (url.pathname === '/conformance-tests' && req.method === 'POST') {
				this.handleConformanceTest(req, res);
				return;
			}
			
			// Handle webhook results endpoint
			if (url.pathname.startsWith('/webhook-test/results/') && req.method === 'GET') {
				this.handleWebhookResults(req, res);
				return;
			}
			
			// Handle webhook endpoint
			if (url.pathname === '/webhook-test') {
				webhookHandler.handleWebhook(req, res);
				return;
			}
			
			// Forward all other requests to control plane
			httpProxy.forward(req, res);
		});
		
		// Setup WebSocket handling
		this.wsProxy = createWebSocketProxy(
			this.controlPlaneUrl,
			this.sdkContractPath,
			this.webhookResults);
		this.wsServer.on('connection', this.wsProxy.handleConnection);
		
		// Start listening
		return new Promise((resolve) => {
			this.httpServer.listen(port, () => {
				console.log(`Protocol proxy listening on port ${port}`);
				resolve();
			});
		});
	}
	
	async stop() {
		return new Promise((resolve) => {
			if (this.httpServer) {
				this.httpServer.close(() => resolve());
			} else {
				resolve();
			}
		});
	}
	
	findConformanceTestCase(conformanceTests, targetId) {
		for (const [ _, test ] of Object.entries(conformanceTests)) {
			if (test?.steps) {
				const step = test?.steps?.find(step => step.id === targetId);
				if (step) {
					return step;
				}
			}
		}
		return null;
	}
	
	async handleConformanceTest(req, res) {
		let body = '';
		for await (const chunk of req) {
			body += chunk;
		}
		
		try {
			const { serviceId, testId } = JSON.parse(body);
			
			await this.waitForServiceConnection(serviceId);
			
			// Load test scenario from contract
			const conformanceTests = this.validator.contract['x-conformance-tests'];
			const testCase = this.findConformanceTestCase(conformanceTests, testId)
			
			if (!testCase) {
				res.writeHead(404);
				res.end(JSON.stringify({ error: `Test scenario ${testId} not found` }));
				return;
			}
			
			// Generate unique test ID
			const testRunId = `${Date.now()}_${Math.random().toString(36).substring(2, 11)}`;
			
			// Initialize webhook results tracking
			this.webhookResults.set(testRunId, {
				status: 'pending',
				results: [],
				testCase
			});
			
			// Execute test steps
			for (const message of this.generateTestMessages(testCase, serviceId, testRunId)) {
				await this.wsProxy.sendToService(serviceId, message);
				// Small delay between messages
				await new Promise(resolve => setTimeout(resolve, 100));
			}
			
			res.writeHead(200);
			res.end(JSON.stringify({ id: testRunId }));
			
		} catch (error) {
			console.error('Conformance test error:', error);
			res.writeHead(500);
			res.end(JSON.stringify({ error: error.message }));
		}
	}
	
	async waitForServiceConnection(serviceId, timeout = 5000) {
		const start = Date.now();
		
		while (Date.now() - start < timeout) {
			if (this.wsProxy.hasActiveConnection(serviceId)) {
				return true;
			}
			await new Promise(resolve => setTimeout(resolve, 100));
		}
		
		throw new Error(`Timeout waiting for service ${serviceId} to connect`);
	}
	
	* generateTestMessages(testCase, serviceId, testRunId) {
		// Base message structure
		const baseMessage = {
			type: 'task_request',
			id: testRunId,
			serviceId,
			input: testCase?.input
		};
		
		switch (testCase.id) {
			case 'health_check':
				yield {
					...baseMessage,
					executionId: `health_check__${testRunId}`,
					idempotencyKey: `health_check__${testRunId}`
				};
				break;
			
			case 'reconnection':
				yield {
					...baseMessage,
					executionId: `reconnect__${testRunId}`,
					idempotencyKey: `reconnect__${testRunId}`
				};
				break;
			
			case 'message_queueing':
				// Send multiple messages to test queueing
				for (let i = 1; i <= 3; i++) {
					yield {
						...baseMessage,
						executionId: `queue__${testRunId}__${i}`,
						idempotencyKey: `queue__${testRunId}__${i}`,
						input: { ...testCase?.input, message: `sending ${i}` }
					};
				}
				break;
			
			case 'large_payload':
				yield {
					...baseMessage,
					executionId: `large_payload__${testRunId}`,
					idempotencyKey: `large_payload__${testRunId}`
				};
				break;
			
			default:
				// For other tests like exactly_once
				if (testCase?.input?.duplicate) {
					for (let i = 0; i < 3; i++) {
						yield {
							...baseMessage,
							executionId: `exec_test__${testRunId}`,
							idempotencyKey: `idem_test__${testRunId}`
						};
					}
				} else {
					yield {
						...baseMessage,
						executionId: `exec_test__${testRunId}`,
						idempotencyKey: `idem_test__${testRunId}`
					};
				}
		}
	}
	
	handleWebhookResults(req, res) {
		const testId = req.url.split('/').pop();
		const result = this.webhookResults.get(testId);
		
		if (!result) {
			res.writeHead(404);
			res.end(JSON.stringify({ error: 'No results found' }));
			return;
		}
		
		res.writeHead(200);
		res.end(JSON.stringify(result));
	}
}
