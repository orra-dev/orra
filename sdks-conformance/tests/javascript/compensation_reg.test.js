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

describe('Compensation Protocol', () => {
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
	
	test('revertible service registration', async () => {
		service = initService({
			name: 'revertible-test-service',
			orraUrl: TEST_HARNESS_URL,
			orraKey: apiKey
		});
		
		await service.register({
			description: 'A service that supports compensation',
			schema: {
				input: {
					type: 'object',
					properties: {
						data: { type: 'string' }
					}
				},
				output: {
					type: 'object',
					properties: {
						result: { type: 'string' }
					}
				}
			},
			revertible: true,
			revertTTL: 3600000 // 1 hour
		});
		
		expect(service.info.id).toBeTruthy();
		expect(service.info.id).toMatch(/^s_[a-zA-Z0-9]+$/);
		expect(service.info.version).toBe(1);
		
		expect(service.info.revertible).toBe(true);
		expect(service.info.revertTTL).toBe(3600000);
	});
});
