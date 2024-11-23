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
		const client = createClient({
			orraUrl: TEST_HARNESS_URL,
			orraKey: apiKey,
		});
		
		await client.registerService('minimal-test-service', {
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
		expect(client.serviceId).toBeTruthy();
		expect(client.serviceId).toMatch(/^s_[a-zA-Z0-9]+$/);
		
		// Verify version is set
		expect(client.version).toBe(1);
		
		// Clean close
		client.close();
	});
	
	test('service registration persistence', async () => {
		let savedServiceId = null;
		
		// Create first client with persistence
		const client = createClient({
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
		
		await client.registerService('persistent-service', {
			description: "a persistent service",
			schema: {
				input: { type: "object", properties: { entry: { type: "string" } } },
				output: { type: "object", properties: { entry: { type: "string" } } },
			}
		});
		const originalId = client.serviceId;
		client.close();
		
		// Create new client instance
		const newClient = createClient({
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
		
		await newClient.registerService('persistent-service', {
			description: "a persistent service",
			schema: {
				input: { type: "object", properties: { entry: { type: "string" } } },
				output: { type: "object", properties: { entry: { type: "string" } } },
			}
		});
		expect(newClient.serviceId).toBe(originalId);
		expect(newClient.version).toBe(2); // Version increments on re-registration
		
		newClient.close();
	});
	
	test('service registration error cases', async () => {
		const client = createClient({
			orraUrl: TEST_HARNESS_URL,
			orraKey: apiKey
		});
		
		// Test invalid schema
		await expect(client.registerService('invalid-schema-service', {
			schema: {
				input: { invalid: true }
			}
		})).rejects.toThrow();
		
		// Test empty service name
		await expect(client.registerService('')).rejects.toThrow();
		
		// Clean up
		client.close();
		
		// Test invalid API key
		const invalidClient = createClient({
			orraUrl: TEST_HARNESS_URL,
			orraKey: 'sk-orra-invalid-key'
		});
		
		await expect(invalidClient.registerService('invalid-auth-service')).rejects.toThrow();
		invalidClient.close();
	});
	
	test('verify finality of close()', async () => {
		const client = createClient({
			orraUrl: TEST_HARNESS_URL,
			orraKey: apiKey
		});
		
		await client.registerService('closing-service', {
			description: "a closing service",
			schema: {
				input: { type: "object", properties: { entry: { type: "string" } } },
				output: { type: "object", properties: { entry: { type: "string" } } },
			}
		});
		client.close();
		
		// Attempt to register after close should throw
		await expect(client.registerService('reopened-service')).rejects.toThrow();
	});
});
