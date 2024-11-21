/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import { expect, test, describe, beforeAll, afterEach } from '@jest/globals';
import { createClient } from '@orra.dev/sdk';
import { rm } from 'fs/promises';
import { join } from 'path';
import { existsSync } from 'fs';

const PROXY_URL = process.env.PROXY_URL || 'http://localhost:8006';
const WEBHOOK_URL = process.env.WEBHOOK_URL || `http://localhost:8006/webhook-test`;
const DEFAULT_ORRA_DIR = '.orra-data';

async function registerProject() {
	const response = await fetch(`${PROXY_URL}/register/project`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json'
		},
		body: JSON.stringify({ webhooks: [WEBHOOK_URL] })
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

describe('Exactly-Once Execution Protocol', () => {
	let apiKey;
	let projectId;
	let client;
	let executionCount = 0;
	
	beforeAll(async () => {
		const registration = await registerProject();
		apiKey = registration.apiKey;
		projectId = registration.projectId;
	});
	
	afterEach(async () => {
		if (client) {
			client.close();
		}
		
		const orraDataPath = join(process.cwd(), DEFAULT_ORRA_DIR);
		if (existsSync(orraDataPath)) {
			await rm(orraDataPath, { recursive: true, force: true });
		}
		
		executionCount = 0;
	});
	
	test('exactly once execution conformance', async () => {
		client = createClient({
			orraUrl: PROXY_URL,
			orraKey: apiKey
		});
		
		// Register service with required schema
		await client.registerService('exactly-once-service', {
			description: 'Test service for exactly-once delivery validation',
			schema: {
				input: {
					type: 'object',
					properties: {
						message: { type: 'string' }
					},
					required: ['message']
				},
				output: {
					type: 'object',
					properties: {
						message: { type: 'string' },
						count: { type: 'number' }
					},
					required: ['message', 'count']
				}
			}
		});
		
		// Set up handler that tracks executions
		client.startHandler(async (task) => {
			executionCount++;
			return {
				message: task.input.message,
				count: executionCount
			};
		});
		
		// Trigger conformance test
		const testResponse = await fetch(`${PROXY_URL}/conformance-tests`, {
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
				'Authorization': `Bearer ${apiKey}`
			},
			body: JSON.stringify({
				projectId,
				serviceId: client.serviceId,
				testId: 'exactly_once'
			})
		});
		
		expect(testResponse.ok).toBeTruthy();
		const testResult = await testResponse.json();
		
		// Poll webhook for results
		const result = await poll(async () => {
			const webhookResult = await fetch(
				`${PROXY_URL}/webhook-test/results/${testResult.id}`,
				{
					headers: {
						'Authorization': `Bearer ${apiKey}`
					}
				}
			);
			
			if (webhookResult.ok) {
				const data = await webhookResult.json();
				if (data.status === 'completed') {
					return data;
				}
			}
			return null;
		}, 10000, 1000);
		
		for (const item of result.results) {
			expect(item.type).toBe("task_result");
			expect(item.result.count).toBe(1);
		}
		expect(executionCount).toBe(1);
	}, 15000);
});

const poll = async (fn, timeout = 5000, interval = 500) => {
	const endTime = Date.now() + timeout;
	
	while (Date.now() < endTime) {
		const result = await fn();
		if (result) return result;
		await new Promise(resolve => setTimeout(resolve, interval));
	}
	
	throw new Error('Polling timed out after ' + timeout + 'ms');
};
