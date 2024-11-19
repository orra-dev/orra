/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import { expect, test, describe, beforeAll, afterEach } from '@jest/globals';
import { createClient } from '@orra.dev/sdk'; // The actual Orra SDK

const PROXY_URL = process.env.PROXY_URL || 'http://localhost:8006';
const WEBHOOK_URL = process.env.WEBHOOK_URL || `http://localhost:8006/webhook-test`;

async function registerProject() {
	const response = await fetch(`${PROXY_URL}/register/project`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json'
		},
		body: JSON.stringify({ webhooks: [ WEBHOOK_URL ] })
	});
	
	if (!response.ok) {
		throw new Error(`Failed to register project: ${response.statusText}`);
	}
	
	const project = await response.json();
	return {
		projectId: project.id,
		apiKey: project.apiKey
	};
}

describe('Echo Service', () => {
	let client;
	let apiKey;
	let projectId;
	
	beforeAll(async () => {
		const registration = await registerProject();
		apiKey = registration.apiKey;
		projectId = registration.projectId;
	});
	
	afterEach(() => {
		if (client) {
			client.close();
		}
	});
	
	test('echo service protocol conformance', async () => {
		// Verify registration
		expect(apiKey).toBeTruthy();
		expect(projectId).toBeTruthy();
		
		// Create client
		client = createClient({
			orraUrl: PROXY_URL,
			orraKey: apiKey,
			persistenceOpts: {
				method: 'custom',
				customSave: async (serviceId) => {
				},
				customLoad: async () => {
					return null;
				}
			}
		});
		
		// Register service using SDK
		await client.registerService('Echo Service', {
			description: 'A service that can echo a message sent to it.',
			schema: {
				input: {
					type: 'object',
					properties: {
						message: { type: 'string' }
					},
					required: [ 'message' ]
				},
				output: {
					type: 'object',
					properties: {
						message: { type: 'string' }
					},
					required: [ 'message' ]
				}
			}
		});
		
		// Set up echo handler using SDK
		client.startHandler(async (task) => {
			return {
				message: task.input.message
			};
		});
		
		const orchestrationResponse = await fetch(`${PROXY_URL}/orchestrations`, {
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
				'Authorization': `Bearer ${apiKey}`
			},
			body: JSON.stringify({
				action: {
					type: "echo",
					content: "Echo this message"
				},
				data: [
					{
						field: "message",
						value: "Hello World"
					}
				],
				webhook: WEBHOOK_URL
			})
		});
		
		expect(orchestrationResponse.ok).toBeTruthy();
		const orchestration = await orchestrationResponse.json();
		
		const result = await poll(async () => {
			const resultResponse = await fetch(
				`${PROXY_URL}/orchestrations/inspections/${orchestration.id}`,
				{
					headers: {
						'Authorization': `Bearer ${apiKey}`
					}
				}
			);
			
			if (resultResponse.ok) {
				const resultData = await resultResponse.json();
				if (resultData.status === 'completed') {
					return resultData;
				}
			}
			
			return null;
		},15000, 1000); // Wait up to 10 seconds, checking every second
		
		expect(result.tasks[0].output).toEqual({ message: 'Hello World' });
	}, 20000);
});

const poll = async (fn, timeout = 5000, interval = 500) => {
	const endTime = Date.now() + timeout;
	
	while (Date.now() < endTime) {
		const result = await fn();
		if (result) return result;
		await new Promise((resolve) => setTimeout(resolve, interval));
	}
	
	throw new Error('Polling timed out after ' + timeout + 'ms');
};
