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

export function createWebSocketProxy(controlPlaneUrl, sdkContractPath, webhookResults) {
	const validator = new ProtocolValidator(sdkContractPath);
	const activeConnections = new Map();
	const metrics = new MetricsCollector();
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
				
				console.log("handleConnection - url", url);
				console.log("handleConnection - serviceId", serviceId);
				
				const controlPlaneWs = new WebSocket(controlPlaneUrl + req.url);
				
				controlPlaneWs.on('open', () => {
					console.log('Control plane WebSocket opened');
					const manager = new ConnectionManager(
						serviceId,
						clientWs,
						controlPlaneWs,
						activeConnections,
						metrics,
						validator,
						webhookResults);
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
			
			if (message.type === 'task_request') {
				const simulator = new TaskRequestSimulator(
					serviceId,
					message,
					metrics,
					activeConnections,
					webhookResults
				);
				simulator.activate();
			}
			
			await manager.clientWs.send(JSON.stringify(message));
		}
	};
}
