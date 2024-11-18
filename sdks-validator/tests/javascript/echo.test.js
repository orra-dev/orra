/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import { expect, test, describe, beforeAll, afterEach } from '@jest/globals';
import { createClient } from '@orra.dev/sdk'; // The actual Orra SDK

const PROXY_URL = process.env.PROXY_URL || 'http://localhost:8006';

async function registerProject() {
	console.log('PROXY_URL', PROXY_URL)
	const response = await fetch(`${PROXY_URL}/register/project`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json'
		},
		body: JSON.stringify({})
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
					console.log('Service ID saved:', serviceId);
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
		
		// Trigger echo action through SDK
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
				webhook: "http://localhost:8006/webhook-test"
			})
		});
		
		expect(orchestrationResponse.ok).toBeTruthy();
		const orchestration = await orchestrationResponse.json();
		
		// Wait for orchestration completion
		await new Promise(r => setTimeout(r, 1000));
		
		// Verify result
		const resultResponse = await fetch(
			`${process.env.PROXY_URL || 'http://localhost:8006'}/orchestrations/inspections/${orchestration.id}`,
			{
				headers: {
					'Authorization': `Bearer ${apiKey}`
				}
			}
		);
		
		expect(resultResponse.ok).toBeTruthy();
		const result = await resultResponse.json();
		
		expect(result.status).toBe('completed');
		expect(result.tasks[0].output).toEqual({ message: 'Hello World' });
	});
});
