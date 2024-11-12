/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import express from 'express';
import { createClient } from '@orra.dev/sdk';
import schema from './schema.json' assert { type: 'json' };

import dotenv from 'dotenv';
dotenv.config();

const app = express();
const port = process.env.PORT || 3400;

// Initialize the Orra client with environment-aware persistence
const orra = createClient({
	orraUrl: process.env.ORRA_URL,
	orraKey: process.env.ORRA_API_KEY,
	persistenceOpts: getPersistenceConfig()
});

// Health check
app.get('/health', (req, res) => {
	res.status(200).json({ status: 'healthy' });
});

async function startService() {
	try {
		// Register the echo service with Orra
		await orra.registerService('EchoService', {
			description: 'A simple service that echoes back the first input value it receives.',
			schema
		});
		
		orra.startHandler(async (task) => {
			console.log('Echoing input:', task.id);
			const message = task?.input
			return { echo: message };
		});
		
		console.log('Echo Service started successfully');
	} catch (error) {
		console.error('Failed to start Echo Service:', error);
		process.exit(1);
	}
}

// Start the Express server and the service
app.listen(port, () => {
	console.log(`Server listening on port ${port}`);
	startService().catch(console.error);
});

// Graceful shutdown
process.on('SIGTERM', async () => {
	console.log('SIGTERM received, shutting down gracefully');
	orra.close();
	process.exit(0);
});

// Configure service key persistence based on environment
function getPersistenceConfig () {
	if (process.env.NODE_ENV === 'development') {
		// For local development with Docker, use file persistence with custom path
		return {
			method: 'file',
			filePath: process.env.ORRA_SERVICE_KEY_PATH || '.orra-data/orra-service-key.json'
		};
	}
	
	// For production (Vercel, etc...), use in-memory or other persistence
	return {
		method: 'custom',
		customSave: async (serviceId) => {
			console.log('Service ID saved:', serviceId);
		},
		customLoad: async () => {
			return null;
		}
	};
}
