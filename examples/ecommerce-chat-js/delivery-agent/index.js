/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import { createClient } from '@orra/sdk';
import { runAgent } from "./agent.js";
import dotenv from 'dotenv';

// Load environment variables
dotenv.config();

// Validate environment variables
if (!process.env.ORRA_URL || !process.env.ORRA_API_KEY) {
	console.error('Error: ORRA_URL and ORRA_API_KEY must be set in the environment variables');
	process.exit(1);
}

// Create the Orra SDK client
const orra = createClient({
	orraUrl: process.env.ORRA_URL,
	orraKey: process.env.ORRA_API_KEY,
});

// Service details
const agentName = 'DeliveryAgent';
const agentDescription = 'An agent that helps customers with estimated delivery dates for online shopping.';
const agentSchema = {
	input: {
		type: 'object',
		properties: {
			customerId: { type: 'string' },
			customerName: { type: 'string' },
			customerAddress: { type: 'string' },
			productDescription: { type: 'string' },
			productAvailability: { type: 'string' },
			warehouseAddress: { type: 'string' },
		},
		required: [ 'customerId', 'customerAddress', 'warehouseAddress', 'productAvailability' ]
	},
	output: {
		type: 'object',
		properties: {
			response: { type: 'string' }
		},
		required: [ 'response' ]
	}
};

// Task handler function
async function handleTask(task) {
	// Extract the message from the task input
	const {
		customerId,
		customerName,
		customerAddress,
		productDescription,
		productAvailability,
		warehouseAddress
	} = task.input;
	
	const response = await runAgent({
		customerId,
		customerName,
		customerAddress,
		productDescription,
		productAvailability,
		warehouseAddress,
	})
	
	return { response: response };
}

// Main function to set up and run the service
async function main() {
	try {
		// Register the service
		await orra.registerAgent(agentName, {
			description: agentDescription,
			schema: agentSchema
		});
		
		// Start the task handler
		orra.startHandler(handleTask);
	} catch (error) {
		console.error('Error setting up the agent:', error);
	}
}

// Run the main function
main();

// Handle graceful shutdown
process.on('SIGINT', () => {
	console.log('Shutting down...');
	orra.close();
	process.exit();
});
