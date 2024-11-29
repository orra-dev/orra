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
		headers: { 'Content-Type': 'application/json' },
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

const poll = async (fn, timeout = 5000, interval = 500) => {
	const endTime = Date.now() + timeout;
	while (Date.now() < endTime) {
		const result = await fn();
		if (result) return result;
		await new Promise(resolve => setTimeout(resolve, interval));
	}
	throw new Error('Polling timed out after ' + timeout + 'ms');
};

describe('Connection Resilience Protocol', () => {
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
			service.shutdown();
		}
		
		const orraDataPath = join(process.cwd(), DEFAULT_ORRA_DIR);
		if (existsSync(orraDataPath)) {
			await rm(orraDataPath, { recursive: true, force: true });
		}
	});
	
	test('mid-task disconnect resilience', async () => {
		service = initService({
			name: 'resilient-task-service',
			orraUrl: TEST_HARNESS_URL,
			orraKey: apiKey
		});

		let executionCount = 0;
		await service.register({
			description: 'Service for testing mid-task disconnection resilience',
			schema: {
				input: { type: 'object', properties: { duration: { type: 'number' } } },
				output: { type: 'object', properties: { completed: { type: 'boolean' }, executionCount: { type: 'number' } } }
			}
		});
		
		service.start(async (task) => {
			executionCount++;
			// Simulate long-running task
			await new Promise(resolve => setTimeout(resolve, task.input.duration || 5000));
			return {
				completed: true,
				executionCount
			};
		});

		console.log('service.info.id', service.info.id);
		
		const testResponse = await fetch(`${TEST_HARNESS_URL}/conformance-tests`, {
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
				'Authorization': `Bearer ${apiKey}`
			},
			body: JSON.stringify({
				serviceId: service.info.id,
				testId: 'mid_task_disconnect'
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
		}, 15000, 1000);

		expect(result.status).toBe('completed');

		const taskResult = result.results.find(r => r.type === 'task_result');
		expect(taskResult.result.completed).toBe(true);
		expect(taskResult.result.executionCount).toBe(1);
		expect(executionCount).toBe(1);
	}, 20000);
	
	test('handles websocket disconnect during registration', async () => {
		let registrationAttempts = 0;
		
		service = initService({
			name: 'resilient-service',
			orraUrl: TEST_HARNESS_URL,
			orraKey: apiKey,
			persistenceOpts: {
				method: 'custom',
				customSave: async () => registrationAttempts++,
				customLoad: async () => null
			}
		});
		
		// Enable disconnect for next WebSocket connection
		await fetch(`${TEST_HARNESS_URL}/test-control/enable-disconnect`, {
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
				'Authorization': `Bearer ${apiKey}`
			}
		});
		
		await service.register({
			description: 'Service for testing registration disconnect',
			schema: {
				input: {
					type: 'object',
					properties: { message: { type: 'string' } }
				},
				output: {
					type: 'object',
					properties: { message: { type: 'string' } }
				}
			}
		});
		
		// Verify requirements from spec
		expect(service.info.id).toBeTruthy(); // service_id_present
		expect(registrationAttempts).toBe(1); // single_registration_attempt
	}, 15000);
});
