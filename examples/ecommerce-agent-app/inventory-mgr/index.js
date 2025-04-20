/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import express from 'express';
import { initService } from '@orra.dev/sdk';
import schema from './schema.json' assert { type: 'json' };

import dotenv from 'dotenv';
dotenv.config();

const app = express();
const port = process.env.PORT || 3300;
const shouldDemoRevertFail = process?.env?.DEMO_REVERT_FAIL;

// Initialize the Orra client with environment-aware persistence
const invToolAsSvc = initService({
	name: 'inventory-manager',
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
		// Register the inventory manager tool as a service with Orra
		await invToolAsSvc.register({
			description: 'An inventory manager that looks up and manages ecommerce products availability. ' +
				'Including, updating inventory in real-time as orders are placed',
			revertible: true,
			schema
		});
		
		invToolAsSvc.onRevert(async (task, result, context) => {
			if(shouldDemoRevertFail === 'true') {
				console.log('Configured to demonstrate revert failure.');
				throw Error('Failed to revert inventory product hold from: ' + result?.hold + ' to: ' + false);
			}
			console.log('Reverting inventory product for task:', task.id);
			console.log('Reverting inventory product hold from:', result?.hold, 'to:', false);
			console.log('Reverting inventory for context with reason:', context?.reason, 'payload:', context?.payload);
		})
		
		invToolAsSvc.start(async (task) => {
			console.log('Processing inventory product for task:', task.id);
			return {
				productId: '697d1744-88dd-4139-beeb-b307dfb1a2f9',
				productDescription: task?.input?.productDescription,
				productAvailability: 'AVAILABLE',
				warehouseAddress: 'Unit 1 Cairnrobin Way, Portlethen, Aberdeen AB12 4NJ',
				hold: true
			};
		});
		
		console.log('Inventory Manager started successfully');
	} catch (error) {
		console.error('Failed to start Inventory Manager:', error);
		process.exit(1);
	}
}

// Start the Express server and the inventory manager
app.listen(port, () => {
	console.log(`Server listening on port ${port}`);
	startService().catch(console.error);
});

// Graceful shutdown
process.on('SIGTERM', async () => {
	console.log('SIGTERM received, shutting down gracefully');
	invToolAsSvc.shutdown();
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
