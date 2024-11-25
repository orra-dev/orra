/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import { expect, test, describe, beforeAll, afterEach } from '@jest/globals';
import { initService } from '@orra.dev/sdk';
import { join } from "path";
import { existsSync } from "fs";
import { rm } from "fs/promises";

const TEST_HARNESS_URL = process.env.TEST_HARNESS_URL || 'http://localhost:8006';
const WEBHOOK_URL = process.env.WEBHOOK_URL || `http://localhost:8006/webhook-test`;
const DEFAULT_ORRA_DIR = '.orra-data';

async function registerProject() {
	const response = await fetch(`${TEST_HARNESS_URL}/register/project`, {
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

const poll = async (fn, timeout = 5000, interval = 500) => {
	const endTime = Date.now() + timeout;
	while (Date.now() < endTime) {
		const result = await fn();
		if (result) return result;
		await new Promise(resolve => setTimeout(resolve, interval));
	}
	throw new Error('Polling timed out after ' + timeout + 'ms');
};

describe('Connection Management Protocol', () => {
	let service;
	let apiKey;
	let projectId;
	
	beforeAll(async () => {
		const registration = await registerProject();
		apiKey = registration.apiKey;
		projectId = registration.projectId;
	});
	
	afterEach(async () => {
		if (service) {
			service.close();
		}
		
		const orraDataPath = join(process.cwd(), DEFAULT_ORRA_DIR);
		if (existsSync(orraDataPath)) {
			await rm(orraDataPath, { recursive: true, force: true });
		}
	});
	
	
	test('health check conformance', async () => {
		service = initService({
			name: 'health-check-service',
			orraUrl: TEST_HARNESS_URL,
			orraKey: apiKey
		});
		
		await service.register({
			description: 'Service for testing health checks',
			schema: {
				input: {
					type: 'object',
					properties: {
						message: { type: 'string' }
					}
				},
				output: {
					type: 'object',
					properties: {
						message: { type: 'string' }
					}
				}
			}
		});
		
		service.start(async (task) => {
			return {
				message: task.input.message,
			};
		});
		
		const testResponse = await fetch(`${TEST_HARNESS_URL}/conformance-tests`, {
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
				'Authorization': `Bearer ${apiKey}`
			},
			body: JSON.stringify({
				serviceId: service.info.id,
				testId: 'health_check'
			})
		});
		
		expect(testResponse.ok).toBeTruthy();
		const testResult = await testResponse.json();
		
		const result = await poll(async () => {
			const webhookResult = await fetch(
				`${TEST_HARNESS_URL}/webhook-test/results/${testResult.id}`,
				{
					headers: { 'Authorization': `Bearer ${apiKey}` }
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
		
		expect(result.status).toBe('completed');
	}, 15000);
	
	test('reconnection conformance', async () => {
		service = initService({
			name: 'reconnection-service',
			orraUrl: TEST_HARNESS_URL,
			orraKey: apiKey
		});

		await service.register({
			description: 'Service for testing reconnection',
			schema: {
				input: {
					type: 'object',
					properties: {
						message: { type: 'string' }
					}
				},
				output: {
					type: 'object',
					properties: {
						message: { type: 'string' }
					}
				}
			}
		});

		service.start(async (task) => {
			return {
				message: task.input.message,
			};
		});

		const testResponse = await fetch(`${TEST_HARNESS_URL}/conformance-tests`, {
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
				'Authorization': `Bearer ${apiKey}`
			},
			body: JSON.stringify({
				serviceId: service.info.id,
				testId: 'reconnection'
			})
		});

		expect(testResponse.ok).toBeTruthy();
		const testResult = await testResponse.json();

		const result = await poll(async () => {
			const webhookResult = await fetch(
				`${TEST_HARNESS_URL}/webhook-test/results/${testResult.id}`,
				{
					headers: { 'Authorization': `Bearer ${apiKey}` }
				}
			);
			
			if (webhookResult.ok) {
				const data = await webhookResult.json();
				if (data.status === 'completed') {
					return data;
				}
			}
			return null;
		}, 35000, 1000);

		expect(result.status).toBe('completed');
	}, 40000);
	
	test('message queueing conformance', async () => {
		service = initService({
			name: 'queue-service',
			orraUrl: TEST_HARNESS_URL,
			orraKey: apiKey
		});

		let messageCount = 0;
		await service.register({
			description: 'Service for testing message queueing',
			schema: {
				input: {
					type: 'object',
					properties: {
						message: { type: 'string' }
					}
				},
				output: {
					type: 'object',
					properties: {
						message: { type: 'string' },
						order: { type: 'number' }
					}
				}
			}
		});

		service.start(async (task) => {
			messageCount++;
			return {
				message: task.input.message,
				sequence: messageCount
			};
		});

		const testResponse = await fetch(`${TEST_HARNESS_URL}/conformance-tests`, {
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
				'Authorization': `Bearer ${apiKey}`
			},
			body: JSON.stringify({
				serviceId: service.info.id,
				testId: 'message_queueing'
			})
		});

		expect(testResponse.ok).toBeTruthy();
		const testResult = await testResponse.json();

		const result = await poll(async () => {
			const webhookResult = await fetch(
				`${TEST_HARNESS_URL}/webhook-test/results/${testResult.id}`,
				{
					headers: { 'Authorization': `Bearer ${apiKey}` }
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

		expect(result.status).toBe('completed');
		expect(result.results.length).toBeGreaterThan(0);

		// Verify message ordering is preserved
		const sequences = result.results.map(r => r.sequence);
		const sortedSequences = [...sequences].sort((a, b) => a - b);
		expect(sequences).toEqual(sortedSequences);
	}, 15000);
});
