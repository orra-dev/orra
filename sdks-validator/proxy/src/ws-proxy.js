/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import WebSocket from 'ws';
import { ProtocolValidator } from './validator.js';

export function createWebSocketProxy(controlPlaneUrl, sdkContractPath) {
	const validator = new ProtocolValidator(sdkContractPath);
	
	return {
		handleConnection: (clientWs, req) => {
			const url = new URL(req.url, `ws://localhost`);
			
			try {
				// Validate connection params
				validator.validateConnection({
					serviceId: url.searchParams.get('serviceId'),
					apiKey: url.searchParams.get('apiKey')
				});
				
				// Connect to control plane
				const controlPlaneWs = new WebSocket(controlPlaneUrl + req.url);
				
				// Handle control plane connection
				controlPlaneWs.on('open', () => {
					// Proxy messages in both directions
					clientWs.on('message', (data) => {
						try {
							const msg = JSON.parse(data.toString());
							validator.validateMessage(msg, 'sdk-outbound');
							controlPlaneWs.send(JSON.stringify(msg));
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
				clientWs.on('close', () => controlPlaneWs.close());
				controlPlaneWs.on('close', () => clientWs.close());
				
			} catch (error) {
				clientWs.close(4000, error.message);
			}
		}
	};
}
