/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import { expect, test, describe, beforeAll, afterEach } from '@jest/globals';
import { initService } from '@orra.dev/sdk';
import { rm } from 'fs/promises';
import { join } from 'path';
import { existsSync } from 'fs';

const TEST_HARNESS_URL = process.env.TEST_HARNESS_URL || 'http://localhost:8006';
const DEFAULT_ORRA_DIR = '.orra-data';

async function registerProject() {
	const response = await fetch(`${TEST_HARNESS_URL}/register/project`, {
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

describe('Service Registration Protocol', () => {
	let apiKey;
	let projectId;
	
	beforeAll(async () => {
		const registration = await registerProject();
		apiKey = registration.apiKey;
		projectId = registration.projectId;
	});
	
	afterEach(async () => {
		// Clean up the default persistence directory if it exists
		const orraDataPath = join(process.cwd(), DEFAULT_ORRA_DIR);
		if (existsSync(orraDataPath)) {
			await rm(orraDataPath, { recursive: true, force: true });
		}
	});
	
	test('minimal service registration', async () => {
		const service = initService({
			name: 'minimal-test-service',
			orraUrl: TEST_HARNESS_URL,
			orraKey: apiKey,
		});
		
		await service.register({
			description: "a minimal service",
			schema: {
				input: {
					type: "object",
					properties: {
						entry: { type: "string" }
					}
				},
				output: {
					type: "object",
					properties: {
						entry: { type: "string" }
					}
				}
			}
		});
		
		// Verify service ID format
		expect(service.info.id).toBeTruthy();
		expect(service.info.id).toMatch(/^s_[a-zA-Z0-9]+$/);
		
		// Verify version is set
		expect(service.info.version).toBe(1);
		
		// Clean close
		service.shutdown();
	});
	
	test('service registration persistence', async () => {
		let savedServiceId = null;
		
		// Create first service with persistence
		const serviceOne = initService({
			name: 'persistent-service',
			orraUrl: TEST_HARNESS_URL,
			orraKey: apiKey,
			persistenceOpts: {
				method: 'custom',
				customSave: async (id) => {
					savedServiceId = id;
				},
				customLoad: async () => savedServiceId
			}
		});
		
		await serviceOne.register({
			description: "a persistent service",
			schema: {
				input: { type: "object", properties: { entry: { type: "string" } } },
				output: { type: "object", properties: { entry: { type: "string" } } },
			}
		});
		const originalId = serviceOne.info.id;
		serviceOne.shutdown();
		
		// Create new service instance
		const serviceTwo = initService({
			name: 'persistent-service',
			orraUrl: TEST_HARNESS_URL,
			orraKey: apiKey,
			persistenceOpts: {
				method: 'custom',
				customSave: async (serviceId) => {
					savedServiceId = serviceId;
				},
				customLoad: async () => savedServiceId
			}
		});
		
		await serviceTwo.register({
			description: "a persistent service",
			schema: {
				input: { type: "object", properties: { entry: { type: "string" } } },
				output: { type: "object", properties: { entry: { type: "string" } } },
			}
		});
		expect(serviceTwo.info.id).toBe(originalId);
		expect(serviceTwo.info.version).toBe(2); // Version increments on re-registration
		
		serviceTwo.shutdown();
	});
	
	test('service init throws error for empty name', () => {
		expect(() => initService({ name: '' })).toThrow();
	});
	
	test('service registration error cases', async () => {
		const invalidSchema = initService({
			name: 'invalid-schema-service',
			orraUrl: TEST_HARNESS_URL,
			orraKey: apiKey
		});
		
		// Test invalid schema
		await expect(invalidSchema.register({
			schema: {
				input: { invalid: true }
			}
		})).rejects.toThrow();
		
		// Clean up
		invalidSchema.shutdown();
		
		// Test invalid API key
		const invalidService = initService({
			name: 'invalid-svc',
			orraUrl: TEST_HARNESS_URL,
			orraKey: 'sk-orra-invalid-key'
		});
		
		await expect(invalidService.register({})).rejects.toThrow();
		invalidService.shutdown();
	});
	
	test('verify finality of close()', async () => {
		const service = initService({
			name: 'closing-service',
			orraUrl: TEST_HARNESS_URL,
			orraKey: apiKey
		});
		
		await service.register( {
			description: "a closing service",
			schema: {
				input: { type: "object", properties: { entry: { type: "string" } } },
				output: { type: "object", properties: { entry: { type: "string" } } },
			}
		});
		service.shutdown();
		
		// Attempt to register after close should throw
		await expect(service.register({})).rejects.toThrow();
	});
});
