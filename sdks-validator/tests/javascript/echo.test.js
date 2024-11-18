/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import { test } from 'node:test';
import assert from 'node:assert';
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

test('echo service protocol conformance', async (t) => {
	const { projectId, apiKey } = await registerProject();
	assert.ok(apiKey, 'Should receive API key');
	assert.ok(projectId, 'Should receive project ID');
	
	const client = createClient({
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
	
	assert.ok(orchestrationResponse.ok, 'Orchestration request should succeed');
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
	
	assert.ok(resultResponse.ok, 'Should be able to fetch orchestration result');
	const result = await resultResponse.json();
	
	assert.equal(result.status, 'completed', 'Orchestration should complete');
	assert.deepEqual(
		result.tasks[0].output,
		{ message: 'Hello World' },
		'Should echo back the message'
	);
	
	// Cleanup
	client.close();
});

process.on('unhandledRejection', (err) => {
	console.error('Unhandled Rejection:', err);
	process.exitCode = 1;
	throw err;
});
