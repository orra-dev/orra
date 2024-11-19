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

export class ProtocolProxy {
	constructor(controlPlaneUrl, sdkContractPath) {
		this.controlPlaneUrl = controlPlaneUrl;
		this.sdkContractPath = sdkContractPath
		this.httpServer = null;
		this.wsServer = null;
		
		// Track webhooks for orchestrations
		this.webhookResults = new Map();
	}
	
	async start(port = 8006) {
		// Create HTTP server first
		this.httpServer = createServer();
		
		// Create WebSocket server attached to HTTP server
		this.wsServer = new WebSocketServer({ server: this.httpServer });
		
		// Setup HTTP routing
		const httpProxy = createHttpProxy(this.controlPlaneUrl, this.sdkContractPath);
		const webhookHandler = createWebhookHandler(this.webhookResults);
		
		// Handle HTTP requests
		this.httpServer.on('request', (req, res) => {
			const url = new URL(req.url, `http://${req.headers.host}`);
			
			// Handle webhook endpoint
			if (url.pathname === '/webhook-test') {
				webhookHandler.handleWebhook(req, res);
				return;
			}
			
			// Forward all other requests to control plane
			httpProxy.forward(req, res);
		});
		
		// Setup WebSocket handling
		const wsProxy = createWebSocketProxy(this.controlPlaneUrl, this.sdkContractPath);
		this.wsServer.on('connection', wsProxy.handleConnection);
		
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
	
	getWebhookResult(orchestrationId) {
		return this.webhookResults.get(orchestrationId);
	}
}
