/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import WebSocket from 'ws';
import { ProtocolValidator } from './validator.js';
import { MetricsCollector } from './metrics.js';

export function createWebSocketProxy(controlPlaneUrl, sdkContractPath, webhookResults) {
	const validator = new ProtocolValidator(sdkContractPath);
	const activeConnections = new Map();
	const metrics = new MetricsCollector();
	let shouldDisconnectNext = false;
	
	class ConnectionManager {
		constructor(serviceId, clientWs, controlPlaneWs) {
			this.serviceId = serviceId;
			this.clientWs = clientWs;
			this.controlPlaneWs = controlPlaneWs;
		}
		
		setupMessageHandlers() {
			this.clientWs.on('message', data => this.handleClientMessage(data));
			this.controlPlaneWs.on('message', data => this.handleControlPlaneMessage(data));
			this.clientWs.on('close', () => this.handleClose());
			this.controlPlaneWs.on('close', () => this.clientWs.close());
		}
		
		handleClientMessage(data) {
			try {
				const msg = JSON.parse(data.toString());
				const payload = msg.payload;
				validator.validateMessage(msg, 'sdk-outbound');
				
				if (payload.type === 'pong') {
					metrics.endOperation(this.serviceId, 'health_check');
					this.updateTestResults('health_check', true);
				} else if (this.isTestResult(payload)) {
					this.handleTestResult(payload);
				} else {
					this.controlPlaneWs.send(JSON.stringify(msg));
				}
			} catch (error) {
				this.handleError(error);
			}
		}
		
		handleControlPlaneMessage(data) {
			try {
				const msg = JSON.parse(data.toString());
				validator.validateMessage(msg, 'sdk-inbound');
				this.clientWs.send(JSON.stringify(msg));
			} catch (error) {
				console.error(`Protocol violation: ${error.message}`);
			}
		}
		
		handleClose() {
			activeConnections.delete(this.serviceId);
			this.controlPlaneWs.close();
			metrics.reset(this.serviceId);
		}
		
		isTestResult(payload) {
			return payload.type === 'task_result' &&
				(
					payload.executionId.includes('exec_test_') ||
					payload.executionId.includes('queue__') ||
					payload.executionId.includes('large_payload__') ||
					payload.executionId.includes('mid_task__')
				);
		}
		
		handleTestResult(payload) {
			const testId = payload.executionId.split('__')[1];
			const testResult = webhookResults.get(testId);
			if (testResult) {
				testResult.results.push(payload);
				testResult.status = 'completed';
				webhookResults.set(testId, testResult);
			}
		}
		
		handleError(error) {
			this.clientWs.close(4002, error.message);
			console.error(`Protocol violation: ${error.message}`);
		}
		
		updateTestResults(testType, success) {
			const testId = metrics.getMetrics(this.serviceId).currentTestId;
			if (testId) {
				const testResult = webhookResults.get(testId);
				if (testResult) {
					testResult.results.push({
						type: testType,
						success,
						timestamp: new Date().toISOString()
					});
					if (success) {
						testResult.status = 'completed';
						webhookResults.set(testId, testResult);
					}
				}
			}
		}
		
		simulateDisconnect() {
			this.clientWs.terminate();
			return true;
		}
	}
	
	class TestMessageHandler {
		constructor(serviceId, message, metrics, activeConnections, webhookResults) {
			this.serviceId = serviceId;
			this.message = message;
			this.metrics = metrics;
			this.activeConnections = activeConnections;
			this.webhookResults = webhookResults;
			this.testId = message.executionId.split('__')[1];
		}
		
		handle() {
			const testType = this.getTestType();
			if (testType) {
				this.metrics.recordMetric(this.serviceId, 'currentTestId', this.testId);
				this[`handle${testType}`]();
			}
		}
		
		getTestType() {
			const execId = this.message.executionId;
			if (execId.startsWith('health_check__')) return 'HealthCheck';
			if (execId.startsWith('reconnect__')) return 'Reconnect';
			if (execId.startsWith('queue__')) return 'Queue';
			if (execId.startsWith('large_payload__')) return 'LargePayload';
			if (execId.startsWith('mid_task__')) return 'MidTaskDisconnect';
			return null;
		}
		
		handleHealthCheck() {
			this.metrics.startOperation(this.serviceId, 'health_check');
			
			// Send ping message
			const pingMessage = {
				type: 'ping',
				serviceId: this.serviceId
			};
			
			const conn = this.activeConnections.get(this.serviceId);
			if (conn) {
				conn.clientWs.send(JSON.stringify(pingMessage));
				
				// Set timeout for pong response
				setTimeout(() => {
					const testResult = this.webhookResults.get(this.testId);
					if (testResult && !testResult.results.some(r => r.type === 'pong')) {
						testResult.status = 'failed';
						testResult.results.push({
							type: 'health_check',
							success: false,
							timestamp: new Date().toISOString()
						});
					}
				}, 5000);
			}
		}
		
		handleReconnect() {
			const conn = this.activeConnections.get(this.serviceId);
			if (!conn) return;
			
			// Force initial disconnect
			setTimeout(() => {
				if (conn.simulateDisconnect()) {
					this.recordDisconnectEvent();
					
					// Monitor for SDK reconnection attempts
					let reconnectionAttempts = 0;
					const monitorReconnection = setInterval(() => {
						reconnectionAttempts++;
						const reconnectedConnection = this.activeConnections.get(this.serviceId);
						const isReconnected = reconnectedConnection?.clientWs.readyState === WebSocket.OPEN;
						
						if (isReconnected) {
							this.recordReconnectionSuccess(reconnectionAttempts);
							clearInterval(monitorReconnection);
							return;
						}
						
						if (reconnectionAttempts * 1000 > 30000) {
							this.recordReconnectionFailure(reconnectionAttempts);
							clearInterval(monitorReconnection);
						}
					}, 1000);
				}
			}, 1000);
		}
		
		recordReconnectionSuccess(attempts) {
			const testResult = this.webhookResults.get(this.testId);
			if (testResult) {
				testResult.results.push({
					type: 'reconnect',
					success: true,
					attempts,
					timestamp: new Date().toISOString()
				});
				testResult.status = 'completed';
				this.webhookResults.set(this.testId, testResult);
			}
		}
		
		recordReconnectionFailure(attempts) {
			const testResult = this.webhookResults.get(this.testId);
			if (testResult) {
				testResult.results.push({
					type: 'reconnect',
					success: false,
					attempts,
					timestamp: new Date().toISOString()
				});
				testResult.status = 'failed';
				this.webhookResults.set(this.testId, testResult);
			}
		}
		
		handleQueue() {
			const initialConnection = this.activeConnections.get(this.serviceId);
			if (!initialConnection) return;
			
			const sequenceNum = parseInt(this.message.executionId.split('__')[2]);
			
			if (sequenceNum === 1) {
				// Force disconnect after first message
				setTimeout(() => {
					if (initialConnection.simulateDisconnect()) {
						this.recordDisconnectEvent();
					}
				}, 500);
			} else if (sequenceNum === 2) {
				// Message should be queued by SDK while disconnected
				this.metrics.recordMetric(this.serviceId, 'queuedMessage', sequenceNum);
			} else if (sequenceNum === 3) {
				// By message 3, check if SDK has reconnected
				const reconnectedConnection = this.activeConnections.get(this.serviceId);
				const hasReconnected = reconnectedConnection?.clientWs.readyState === WebSocket.OPEN;
				
				if (hasReconnected) {
					this.verifyMessageOrder();
				} else {
					this.recordTestFailure('SDK failed to reconnect');
				}
			}
		}
		
		verifyMessageOrder() {
			const testResult = this.webhookResults.get(this.testId);
			if (!testResult) return;
			
			const messageOrder = testResult.results
				.filter(r => r.type === 'task_result')
				.map(r => r.result.sequence);
			
			const isOrdered = messageOrder.every((num, idx) =>
				idx === 0 || num > messageOrder[idx - 1]
			);
			
			testResult.results.push({
				type: 'message_ordering',
				success: isOrdered,
				messageOrder,
				expectedOrder: [ 1, 2, 3 ],
				timestamp: new Date().toISOString()
			});
			testResult.status = isOrdered ? 'completed' : 'failed';
			this.webhookResults.set(this.testId, testResult);
		}
		
		handleLargePayload() {
			// Generate large payload
			const msgContent = this.message?.input?.message;
			const msgSize = this.message?.input?.size;
			
			const payload = msgContent?.repeat(msgSize);
			const testMessage = {
				...this.message,
				input: {
					message: payload,
					size: msgSize
				}
			};
			
			const conn = this.activeConnections.get(this.serviceId);
			if (!conn) return;
			
			console.log('SENDING!');
			conn.clientWs.send(JSON.stringify(testMessage));
			
			// Monitor response size
			this.monitorResponseSize();
		}
		
		monitorResponseSize() {
			const timeoutId = setTimeout(() => {
				const testResult = this.webhookResults.get(this.testId);
				if (!testResult || testResult.status !== 'completed') {
					testResult.status = 'failed';
					testResult.results.push({
						type: 'large_payload',
						error: 'Response size verification failed',
						timestamp: new Date().toISOString()
					});
					this.webhookResults.set(this.testId, testResult);
				}
			}, 10000);
			
			// Clean up timeout if test completes
			const testResult = this.webhookResults.get(this.testId);
			if (testResult) {
				testResult.cleanup = () => clearTimeout(timeoutId);
			}
		}
		
		recordTestFailure(reason) {
			const testResult = this.webhookResults.get(this.testId);
			if (!testResult) return;
			
			testResult.results.push({
				type: 'test_failure',
				reason,
				timestamp: new Date().toISOString()
			});
			testResult.status = 'failed';
			webhookResults.set(this.testId, testResult);
		}
		
		handleMidTaskDisconnect() {
			const conn = this.activeConnections.get(this.serviceId);
			if (!conn) return;
			
			// Record start of long task
			this.recordTestEvent('long_task_started');
			this.message
			// Wait 1000ms then force disconnect
			setTimeout(() => {
				if (conn.simulateDisconnect()) {
					this.recordTestEvent('connection_dropped');
					
					// Wait 2000ms then restore connection
					setTimeout(() => {
						// Connection will be restored automatically by SDK
						this.recordTestEvent('connection_restored');
						
						// Start monitoring for task completion
						this.monitorTaskCompletion();
					}, 2000);
				}
			}, 1000);
		}
		
		monitorTaskCompletion() {
			const timeoutId = setTimeout(() => {
				const testResult = this.webhookResults.get(this.testId);
				if (!testResult || testResult.status !== 'completed') {
					testResult.status = 'failed';
					testResult.results.push({
						type: 'task_completion',
						error: 'Task did not complete after reconnection',
						timestamp: new Date().toISOString()
					});
					this.webhookResults.set(this.testId, testResult);
				}
			}, 10000);
			
			// Clean up timeout if test completes
			const testResult = this.webhookResults.get(this.testId);
			if (testResult) {
				testResult.cleanup = () => clearTimeout(timeoutId);
			}
		}
		
		recordTestEvent(eventType) {
			const testResult = this.webhookResults.get(this.testId);
			if (testResult) {
				testResult.results.push({
					type: eventType,
					timestamp: new Date().toISOString()
				});
				this.webhookResults.set(this.testId, testResult);
			}
		}
		
		recordDisconnectEvent() {
			const testResult = this.webhookResults.get(this.testId);
			if (testResult) {
				testResult.results.push({
					type: 'disconnect',
					timestamp: new Date().toISOString()
				});
				this.webhookResults.set(this.testId, testResult);
			}
		}
	}
	
	return {
		handleConnection: (clientWs, req) => {
			try {
				const url = new URL(req.url, `ws://localhost`);
				const serviceId = url.searchParams.get('serviceId');
				
				validator.validateConnection({
					serviceId,
					apiKey: url.searchParams.get('apiKey')
				});
				
				if (shouldDisconnectNext) {
					shouldDisconnectNext = false;
					console.log(`Test-triggered WebSocket disconnect for service: ${serviceId}`);
					clientWs.terminate(); // Force disconnect instead of clean close
					return;
				}
				
				const controlPlaneWs = new WebSocket(controlPlaneUrl + req.url);
				
				controlPlaneWs.on('open', () => {
					const manager = new ConnectionManager(serviceId, clientWs, controlPlaneWs);
					activeConnections.set(serviceId, manager);
					manager.setupMessageHandlers();
				});
				
			} catch (error) {
				clientWs.close(4000, error.message);
			}
		},
		
		enableDisconnectNext: () => {
			shouldDisconnectNext = true;
		},
		
		hasActiveConnection: (serviceId) => {
			const manager = activeConnections.get(serviceId);
			return manager && manager.clientWs.readyState === WebSocket.OPEN;
		},
		
		sendToService: async (serviceId, message) => {
			const manager = activeConnections.get(serviceId);
			if (!manager || manager.clientWs.readyState !== WebSocket.OPEN) {
				throw new Error(`No active connection for service ${serviceId}`);
			}
			
			if (message.type === 'task_request') {
				const handler = new TestMessageHandler(
					serviceId,
					message,
					metrics,
					activeConnections,
					webhookResults
				);
				handler.handle();
			}
			
			await manager.clientWs.send(JSON.stringify(message));
		}
	};
}
