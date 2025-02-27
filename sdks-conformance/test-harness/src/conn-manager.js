/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

export class ConnectionManager {
	constructor(serviceId, clientWs, planEngineWs, activeConnections, metrics, validator, webhookResults, compensationTestManager) {
		this.serviceId = serviceId;
		this.clientWs = clientWs;
		this.planEngineWs = planEngineWs;
		this.activeConnections = activeConnections;
		this.metrics = metrics;
		this.validator = validator;
		this.webhookResults = webhookResults;
		this.compensationTestManager = compensationTestManager;
	}
	
	setupMessageHandlers() {
		this.clientWs.on('error', (error) => {
			console.log('Client WebSocket error:', error);
		});
		
		this.planEngineWs.on('error', (error) => {
			console.log('Control plane WebSocket error:', error);
		});
		
		console.log('Control plane WebSocket readyState:', this.planEngineWs.readyState);
		console.log('Client WebSocket readyState:', this.clientWs.readyState);
		
		this.clientWs.on('message', data => {
			this.handleClientMessage(data)
		});
		
		this.planEngineWs.on('message', data => {
			this.handleControlPlaneMessage(data)
		});
		
		this.clientWs.on('close', () => {
			console.log('Client WebSocket closed');
			this.handleClose()
		});
		
		this.planEngineWs.on('close', () => {
			console.log('Control plane WebSocket closed');
			this.clientWs.close()
		});
	}
	
	handleClientMessage(data) {
		try {
			const msg = JSON.parse(data.toString());
			const payload = msg.payload;
			this.validator.validateMessage(msg, 'sdk-outbound');
			
			if (payload.type === 'pong') {
				this.metrics.endOperation(this.serviceId, 'health_check');
				this.updateTestResults('health_check', true);
			} else if (this.isTestResult(payload)) {
				this.handleTestResult(payload);
			} else if (this.isCompensationFlowResult(payload)) {
				this.handleCompensationTestResult(payload);
			} else {
				this.planEngineWs.send(JSON.stringify(msg));
			}
		} catch (error) {
			this.handleError(error);
		}
	}
	
	handleControlPlaneMessage(data) {
		try {
			const msg = JSON.parse(data.toString());
			this.validator.validateMessage(msg, 'sdk-inbound');
			this.clientWs.send(JSON.stringify(msg));
		} catch (error) {
			console.error(`Protocol violation: ${error.message}`);
		}
	}
	
	handleClose() {
		this.activeConnections.delete(this.serviceId);
		this.planEngineWs.close();
		this.metrics.reset(this.serviceId);
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
	
	isCompensationFlowResult(payload) {
		return payload.type === 'task_result' && payload.executionId.includes('comp_test_');
	}
	
	handleTestResult(payload) {
		const testId = payload.executionId.split('__')[1];
		const testResult = this.webhookResults.get(testId);
		
		if(!testResult) return;
		
		testResult.results.push(payload);
		testResult.status = 'completed';
		this.webhookResults.set(testId, testResult);
	}
	
	handleCompensationTestResult(payload) {
		const testId = payload.executionId.split('__')[1];
		const testResult = this.webhookResults.get(testId);
		
		if(!testResult) return;
		const state = this.compensationTestManager.handleTaskResult(testId, payload);
		if (state === 'completed') {
			testResult.status = 'completed';
			this.webhookResults.set(testId, testResult);
			this.compensationTestManager.reset()
		}
		const afterTestResult = this.webhookResults.get(testId);
		console.log(`handleCompensationTestResult afterTestResult`, JSON.stringify(afterTestResult));
	}
	
	handleError(error) {
		this.clientWs.close(4002, error.message);
		console.error(`Protocol violation: ${error.message}`);
	}
	
	updateTestResults(testType, success) {
		const testId = this.metrics.getMetrics(this.serviceId).currentTestId;
		if (testId) {
			const testResult = this.webhookResults.get(testId);
			if (testResult) {
				testResult.results.push({
					type: testType,
					success,
					timestamp: new Date().toISOString()
				});
				if (success) {
					testResult.status = 'completed';
					this.webhookResults.set(testId, testResult);
				}
			}
		}
	}
	
	clientWsSimulateDisconnect() {
		this.clientWs.terminate();
		return true;
	}
}
