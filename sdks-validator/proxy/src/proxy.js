/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import WebSocket, { WebSocketServer } from 'ws';
import { ProtocolValidator } from './validator.js';
import { MetricsCollector } from './metrics.js';
import { URL } from 'url';

export class ProtocolProxy {
	constructor(controlPlaneUrl) {
		this.controlPlaneUrl = controlPlaneUrl;
		this.validator = new ProtocolValidator();
		this.metrics = new MetricsCollector();
		this.server = null;
	}
	
	async start(port = 8006) {
		this.server = new WebSocketServer({ port });
		
		this.server.on('connection', (clientWs, req) => {
			const params = new URL(req.url, 'ws://localhost').searchParams;
			const serviceId = params.get('serviceId');
			
			try {
				// Validate connection parameters
				this.validator.validateConnection({
					serviceId,
					apiKey: params.get('apiKey')
				});
				
				// Start measuring connection setup
				this.metrics.startOperation(serviceId, 'connection');
				
				// Connect to real control plane
				const controlPlaneWs = new WebSocket(this.controlPlaneUrl + req.url);
				
				controlPlaneWs.on('open', () => {
					// Validate connection timing
					const duration = this.metrics.endOperation(serviceId, 'connection');
					this.validator.validateTiming('connection', duration);
					
					this.setupMessageProxy(serviceId, clientWs, controlPlaneWs);
				});
				
				controlPlaneWs.on('error', (error) => {
					clientWs.close(4001, error.message);
				});
				
			} catch (error) {
				clientWs.close(4000, error.message);
			}
		});
		
		return new Promise((resolve) => {
			this.server.on('listening', () => resolve());
		});
	}
	
	setupMessageProxy(serviceId, clientWs, controlPlaneWs) {
		// Client -> Control Plane
		clientWs.on('message', (data) => {
			try {
				const msg = JSON.parse(data.toString());
				
				// Validate outbound message
				this.validator.validateMessage(msg, 'outbound');
				
				if (msg.type === 'task_result') {
					const duration = this.metrics.endOperation(serviceId, 'task_response');
					this.validator.validateTiming('task_response', duration);
				}
				
				// Forward valid message
				controlPlaneWs.send(data);
			} catch (error) {
				clientWs.close(4002, error.message);
			}
		});
		
		// Control Plane -> Client
		controlPlaneWs.on('message', (data) => {
			try {
				const msg = JSON.parse(data.toString());
				
				// Validate inbound message
				this.validator.validateMessage(msg, 'inbound');
				
				if (msg.type === 'task_request') {
					this.metrics.startOperation(serviceId, 'task_response');
				}
				
				// Forward valid message
				clientWs.send(data);
			} catch (error) {
				// Log control plane protocol violations
				console.error('Control plane protocol violation:', error);
				this.metrics.recordMetric(serviceId, 'protocol_violations', error.message);
			}
		});
		
		// Cleanup
		clientWs.on('close', () => {
			controlPlaneWs.close();
			this.metrics.reset(serviceId);
		});
		
		controlPlaneWs.on('close', () => {
			clientWs.close();
			this.metrics.reset(serviceId);
		});
	}
	
	async stop() {
		return new Promise((resolve) => {
			if (this.server) {
				this.server.close(() => resolve());
			} else {
				resolve();
			}
		});
	}
	
	getMetrics(serviceId) {
		return this.metrics.getMetrics(serviceId);
	}
}
