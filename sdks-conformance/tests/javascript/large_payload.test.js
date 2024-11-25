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
const MAX_MESSAGE_SIZE = 10 * 1024 * 1024; // Match proxy's 10MB limit

async function registerProject() {
	const response = await fetch(`${TEST_HARNESS_URL}/register/project`, {
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

const poll = async (fn, timeout = 5000, interval = 500) => {
	const endTime = Date.now() + timeout;
	while (Date.now() < endTime) {
		const result = await fn();
		if (result) return result;
		await new Promise(resolve => setTimeout(resolve, interval));
	}
	throw new Error('Polling timed out after ' + timeout + 'ms');
};

describe('Large Payload Execution Protocol', () => {
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
	
	test('large payload conformance', async () => {
		service = initService({
			name: 'large-payload-service',
			orraUrl: TEST_HARNESS_URL,
			orraKey: apiKey
		});
		
		// Register service with large message handling
		await service.register({
			description: 'Service for testing large payload handling',
			schema: {
				input: {
					type: 'object',
					properties: {
						message: { type: 'string' },
						size: { type: 'number' },
					}
				},
				output: {
					type: 'object',
					properties: {
						validatedSize: { type: 'number' },
						checksum: { type: 'string' }
					}
				}
			}
		});
		
		service.start(async (task) => {
			return {
				validatedSize: task.input.message.length,
				checksum: Buffer.from(task.input.message).toString('base64').slice(0, 10)
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
				testId: 'large_payload'
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
		expect(taskResult.result.validatedSize).toBe(MAX_MESSAGE_SIZE);
	}, 20000);
});
