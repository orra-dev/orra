/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import { request } from 'http';
import { ProtocolValidator } from './validator.js';

export function createHttpProxy(controlPlaneUrl, sdkContractPath) {
	const validator = new ProtocolValidator(sdkContractPath);
	const baseUrl = new URL(controlPlaneUrl);
	
	return {
		forward: async (req, res) => {
			const url = new URL(req.url, `http://${req.headers.host}`);
			const targetUrl = new URL(url.pathname + url.search, baseUrl);
			
			// Validate API key if present
			const authHeader = req.headers['authorization'];
			if (authHeader?.startsWith('Bearer ')) {
				const apiKey = authHeader.split(' ')[1];
				validator.validateApiKey(apiKey);
			}
			
			// Forward the request
			const proxyReq = request(
				targetUrl,
				{
					method: req.method,
					headers: {
						...req.headers,
						host: baseUrl.host
					}
				},
				(proxyRes) => {
					// Copy response headers
					res.writeHead(proxyRes.statusCode, proxyRes.headers);
					proxyRes.pipe(res);
				}
			);
			
			// Forward request body if present
			if (['POST', 'PUT', 'PATCH'].includes(req.method)) {
				req.pipe(proxyReq);
			} else {
				proxyReq.end();
			}
			
			// Handle errors
			proxyReq.on('error', (error) => {
				console.error('Proxy request failed:', error);
				res.statusCode = 502;
				res.end(JSON.stringify({ error: 'Proxy request failed' }));
			});
		}
	};
}
