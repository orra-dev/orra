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

describe('Compensation Execution', () => {
	let service;
	let apiKey;
	let projectId;
	let compensationReceived = null;
	let executionOrder = [];
	
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
		compensationReceived = null;
		executionOrder = [];
	});
	
	test('compensation execution', async () => {
		service = initService({
			name: 'compensation-test-service',
			orraUrl: TEST_HARNESS_URL,
			orraKey: apiKey
		});
		
		await service.register({
			description: 'Service for testing compensation execution',
			schema: {
				input: {
					type: 'object',
					properties: {
						task_data: { type: 'string' },
						shouldSucceed: { type: 'boolean' },
						shouldFail: { type: 'boolean' }
					}
				},
				output: {
					type: 'object',
					properties: {
						processed: { type: 'boolean' },
						data: { type: 'string' }
					}
				}
			},
			revertible: true,
			revertTTL: 3600000 // 1 hour
		});
		
		// Register compensation handler
		service.onRevert((originalTask, taskResult) => {
			executionOrder.push('compensation');
			compensationReceived = { originalTask, taskResult };
			return { status: 'completed' };
		});
		
		// Register task handler
		service.start(async (task) => {
			if (task.input.shouldFail) {
				executionOrder.push('task_failed');
				throw new Error('Task configured to fail');
			}
			
			executionOrder.push('task_succeeded');
			return {
				processed: true,
				data: `Processed: ${task.input.task_data}`
			};
		});
		
		// Trigger test
		const testResponse = await fetch(`${TEST_HARNESS_URL}/conformance-tests`, {
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
				'Authorization': `Bearer ${apiKey}`
			},
			body: JSON.stringify({
				serviceId: service.info.id,
				testId: 'compensation_execution'
			})
		});
		
		expect(testResponse.ok).toBeTruthy();
		const testResult = await testResponse.json();
		
		// Poll for completion
		const result = await poll(async () => {
			const webhookResult = await fetch(
				`${TEST_HARNESS_URL}/webhook-test/results/${testResult.id}`,
				{ headers: { 'Authorization': `Bearer ${apiKey}` } }
			);
			
			if (webhookResult.ok) {
				const data = await webhookResult.json();
				return data.status === 'completed' ? data : null;
			}
			return null;
		}, 10000, 1000);
		
		// Verify execution order
		expect(executionOrder).toEqual([
			'task_succeeded',
			'task_failed',
			'compensation'
		]);
		
		
		
		// Verify compensation data
		expect(compensationReceived).toBeTruthy();
		expect(compensationReceived.originalTask.input.task_data).toBe('test data');
		expect(compensationReceived.taskResult.processed).toBe(true);
		
		// Verify test results
		const successTask = result.results.find(r => r.type === 'first_task_completed');
		expect(successTask).toBeTruthy();
		
		const failedTask = result.results.find(r => r.type === 'second_task_failed');
		expect(failedTask).toBeTruthy();
		
		const compensation = result.results.find(r => r.type === 'compensation_completed');
		expect(compensation).toBeTruthy();
	}, 15000);
	
	test('partial compensation', async () => {
		service = initService({
			name: 'partial-comp-test-service',
			orraUrl: TEST_HARNESS_URL,
			orraKey: apiKey
		});
		
		await service.register({
			description: 'Service for testing partial compensation',
			schema: {
				input: {
					type: 'object',
					properties: {
						operations: {
							type: 'array',
							items: { type: 'string' }
						},
						shouldSucceed: { type: 'boolean' },
						shouldFail: { type: 'boolean' }
					}
				},
				output: {
					type: 'object',
					properties: {
						processed: { type: 'boolean' },
						operations: {
							type: 'array',
							items: { type: 'string' }
						}
					}
				}
			},
			revertible: true,
			revertTTL: 3600000
		});
		
		// Track processed operations for verification
		const completedOperations =  ['op1', 'op2'];
		const remainingOperations = ['op3', 'op4'];
		
		// Register compensation handler that reports partial completion
		service.onRevert((originalTask, taskResult) => {
			executionOrder.push('partial_compensation');
			compensationReceived = { originalTask, taskResult };
			
			// Return partial completion status
			return {
				status: 'partial',
				partial: {
					completed: completedOperations,
					remaining: remainingOperations
				}
			};
		});
		
		// Register task handler
		service.start(async (task) => {
			if (task.input.shouldFail) {
				executionOrder.push('task_failed');
				throw new Error('Task configured to fail');
			}
			
			executionOrder.push('task_succeeded');
			return {
				processed: true,
				operations: task.input.operations
			};
		});
		
		// Trigger test
		const testResponse = await fetch(`${TEST_HARNESS_URL}/conformance-tests`, {
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
				'Authorization': `Bearer ${apiKey}`
			},
			body: JSON.stringify({
				serviceId: service.info.id,
				testId: 'partial_compensation'
			})
		});
		
		expect(testResponse.ok).toBeTruthy();
		const testResult = await testResponse.json();
		
		// Poll for completion
		const result = await poll(async () => {
			const webhookResult = await fetch(
				`${TEST_HARNESS_URL}/webhook-test/results/${testResult.id}`,
				{ headers: { 'Authorization': `Bearer ${apiKey}` } }
			);
			
			if (webhookResult.ok) {
				const data = await webhookResult.json();
				return data.status === 'completed' ? data : null;
			}
			return null;
		}, 15000, 1000);
		
		// Verify execution order
		expect(executionOrder).toEqual([
			'task_succeeded',
			'task_failed',
			'partial_compensation'
		]);
		
		// Verify partial compensation result
		expect(compensationReceived).toBeTruthy();
		expect(compensationReceived.originalTask.input.operations).toEqual(['op1', 'op2', 'op3', 'op4']);
		
		// Verify test results
		const successTask = result.results.find(r => r.type === 'first_task_completed');
		expect(successTask).toBeTruthy();
		
		const failedTask = result.results.find(r => r.type === 'second_task_failed');
		expect(failedTask).toBeTruthy();
		
		const compensation = result.results.find(r => r.type === 'compensation_partial');
		expect(compensation).toBeTruthy();
		expect(compensation.completed).toEqual(completedOperations);
		expect(compensation.remaining).toEqual(remainingOperations);
	}, 15000);
});
