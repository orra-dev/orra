/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import WebSocket from "ws";

export class TaskRequestSimulator {
	constructor(serviceId, message, metrics, activeConnections, webhookResults, compensationTestManager) {
		this.serviceId = serviceId;
		this.message = message;
		this.metrics = metrics;
		this.activeConnections = activeConnections;
		this.webhookResults = webhookResults;
		this.testId = message.executionId.split('__')[1];
		this.compensationTestManager = compensationTestManager
	}
	
	activate() {
		const testType = this.getTestType();
		if (testType) {
			this.metrics.recordMetric(this.serviceId, 'currentTestId', this.testId);
			this[`activate${testType}`]();
		}
	}
	
	getTestType() {
		const execId = this.message.executionId;
		if (execId.startsWith('health_check__')) return 'HealthCheck';
		if (execId.startsWith('reconnect__')) return 'Reconnect';
		if (execId.startsWith('queue__')) return 'Queue';
		if (execId.startsWith('large_payload__')) return 'LargePayload';
		if (execId.startsWith('mid_task__')) return 'MidTaskDisconnect';
		if (execId.startsWith('comp_test_')) return 'CompensationExecution';
		return null;
	}
	
	activateHealthCheck() {
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
	
	activateReconnect() {
		const conn = this.activeConnections.get(this.serviceId);
		if (!conn) return;
		
		// Force initial disconnect
		setTimeout(() => {
			if (conn.clientWsSimulateDisconnect()) {
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
	
	activateQueue() {
		const initialConnection = this.activeConnections.get(this.serviceId);
		if (!initialConnection) return;
		
		const sequenceNum = parseInt(this.message.executionId.split('__')[2]);
		
		if (sequenceNum === 1) {
			// Force disconnect after first message
			setTimeout(() => {
				if (initialConnection.clientWsSimulateDisconnect()) {
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
	
	activateLargePayload() {
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
		this.webhookResults.set(this.testId, testResult);
	}
	
	activateMidTaskDisconnect() {
		const conn = this.activeConnections.get(this.serviceId);
		if (!conn) return;
		
		// Record start of long task
		this.recordTestEvent('long_task_started');
		conn.clientWs.send(JSON.stringify(this.message))
		
		// Wait 1000ms then force disconnect
		setTimeout(() => {
			if (conn.clientWsSimulateDisconnect()) {
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
	
	activateCompensationExecution() {
		const conn = this.activeConnections.get(this.serviceId);
		if (!conn) return;
		
		// // Store the test manager for result handling
		// this.metrics.recordMetric(this.serviceId, 'compensationTestManager', this.compensationTestManager);
		//
		// Process message based on execution ID pattern
		const execId = this.message.executionId;
		if (execId.includes('comp_test_success')) {
			// First task processing
			conn.clientWs.send(JSON.stringify(this.message));
			
		} else if (execId.includes('comp_test_fail')) {
			// Second task (should fail)
			conn.clientWs.send(JSON.stringify(this.message));
			
		} else if (execId.includes('comp_test_revert')) {
			// Compensation request
			const successResult = this.compensationTestManager.getSuccessfulTaskResult();
			if (successResult) {
				this.message.input.taskResult = successResult?.result?.task
				const compensationMsg = {
					...this.message,
					input: {
						originalTask: this.message?.input?.originalTask,
						taskResult: successResult?.result?.task
					}
				};
				this.compensationTestManager.activateCompensationStarted();
				conn.clientWs.send(JSON.stringify(compensationMsg));
			}
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
