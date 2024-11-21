/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import WebSocket from 'ws';
import { ProtocolValidator } from './validator.js';

export function createWebSocketProxy(controlPlaneUrl, sdkContractPath, webhookResults) {
	const validator = new ProtocolValidator(sdkContractPath);
	const activeConnections = new Map();
	
	return {
		handleConnection: (clientWs, req) => {
			try {
				const url = new URL(req.url, `ws://localhost`);
				
				validator.validateConnection({
					serviceId: url.searchParams.get('serviceId'),
					apiKey: url.searchParams.get('apiKey')
				});
				
				// Connect to control plane
				const controlPlaneWs = new WebSocket(controlPlaneUrl + req.url);
				
				// Handle control plane connection
				controlPlaneWs.on('open', () => {
					const serviceId = url.searchParams.get('serviceId');
					activeConnections.set(serviceId, clientWs);
					
					// Proxy messages in both directions
					// clientWs: SDK-OUTBOUND -> Control plane
					// controlPlaneWs: SDK-INBOUND <- Control plane
					
					clientWs.on('message', (data) => {
						try {
							const msg = JSON.parse(data.toString());
							const payload = msg.payload;
							validator.validateMessage(msg, 'sdk-outbound');
							
							// Handle conformance test results
							if (payload.type === 'task_result' && payload.executionId.includes('exec_test_')) {
								const testId = payload.executionId.split('__')[1];
								const testResult = webhookResults.get(testId);
								if (testResult) {
									testResult.results.push(payload);
									testResult.status = 'completed';
									webhookResults.set(testId, testResult)
								}
							} else {
								// Regular message - forward to control plane
								controlPlaneWs.send(JSON.stringify(msg));
							}
						} catch (error) {
							clientWs.close(4002, error.message);
							console.error(`SDK-OUTBOUND -> Control plane protocol violation: ${error.message}`, '<--->',data.toString());
							throw new Error(`SDK-OUTBOUND -> Control plane protocol violation: ${error.message}`)
						}
					});
					
					controlPlaneWs.on('message', (data) => {
						try {
							const msg = JSON.parse(data.toString());
							validator.validateMessage(msg, 'sdk-inbound');
							clientWs.send(JSON.stringify(msg));
						} catch (error) {
							console.error(`SDK-INBOUND <- Control plane protocol violation: ${error.message}`, '<--->',data.toString());
							throw new Error(`SDK-INBOUND <- Control plane protocol violation: ${error.message}`)
						}
					});
				});
				
				// Handle closures
				clientWs.on('close', () => {
					const serviceId = url.searchParams.get('serviceId');
					activeConnections.delete(serviceId);
					controlPlaneWs.close();
				});
				controlPlaneWs.on('close', () => clientWs.close());
				
			} catch (error) {
				clientWs.close(4000, error.message);
			}
		},
		
		hasActiveConnection: (serviceId) => {
			return activeConnections.has(serviceId) &&
				activeConnections.get(serviceId).readyState === WebSocket.OPEN;
		},
		
		sendToService: async (serviceId, message) => {
			const ws = activeConnections.get(serviceId);
			if (!ws || ws.readyState !== WebSocket.OPEN) {
				throw new Error(`No active connection for service ${serviceId}`);
			}
			await ws.send(JSON.stringify(message));
		}
	};
}
