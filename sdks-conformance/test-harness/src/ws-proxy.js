/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import WebSocket from 'ws';
import { ProtocolValidator } from './protocol-validator.js';
import { MetricsCollector } from './metrics.js';
import { ConnectionManager } from './conn-manager.js';
import { TaskRequestSimulator } from './task-request-simulator.js';
import { CompensationTestManager } from "./compensation-test-mgr.js";

function shouldSimulate(message) {
	const execId = message?.executionId;
	const mType = message?.type;
	
	const simulateRequest = mType === 'task_request' || mType === 'compensation_request';
	const simulateTestCase = execId.startsWith('health_check__') ||
		execId.startsWith('reconnect__') ||
		execId.startsWith('queue__') ||
		execId.startsWith('large_payload__') ||
		execId.startsWith('mid_task__') ||
		execId.startsWith('comp_test_');
	
	return simulateRequest && simulateTestCase;
}

export function createWebSocketProxy(planEngineUrl, sdkContractPath, webhookResults) {
	const validator = new ProtocolValidator(sdkContractPath);
	const activeConnections = new Map();
	const metrics = new MetricsCollector();
	const compensationTestManager = new CompensationTestManager(webhookResults);
	
	let shouldDisconnectNext = false;
	
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
				
				const planEngineWs = new WebSocket(planEngineUrl + req.url);
				
				planEngineWs.on('open', () => {
					console.log('Control plane WebSocket opened');
					const manager = new ConnectionManager(
						serviceId,
						clientWs,
						planEngineWs,
						activeConnections,
						metrics,
						validator,
						webhookResults,
						compensationTestManager);
					activeConnections.set(serviceId, manager);
					manager.setupMessageHandlers();
					console.log('Control plane WebSocket setupMessageHandlers completed!');
				});
				
			} catch (error) {
				console.log("handleConnection - error", error.message);
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
			
			if (shouldSimulate(message)) {
				const simulator = new TaskRequestSimulator(
					serviceId,
					message,
					metrics,
					activeConnections,
					webhookResults,
					compensationTestManager);
				return simulator.activate();
			}
			
			await manager.clientWs.send(JSON.stringify(message));
		},
	};
}
