/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import { parse } from 'yaml';
import { readFileSync } from 'fs';
import { join } from 'path';

export class ProtocolValidator {
	#contract;
	
	constructor() {
		const contractPath = join(__dirname, '../../contracts/sdk.yaml');
		const contractYaml = readFileSync(contractPath, 'utf8');
		this.#contract = parse(contractYaml);
	}
	
	validateConnection(params) {
		const { serviceId, apiKey } = params;
		
		// Required parameters
		if (!serviceId || !apiKey) {
			throw new Error('serviceId and apiKey are required');
		}
		
		// Format validation
		if (!serviceId.startsWith('s_')) {
			throw new Error('Invalid serviceId format');
		}
		if (!apiKey.startsWith('sk-orra-')) {
			throw new Error('Invalid apiKey format');
		}
		
		return true;
	}
	
	validateMessage(msg, direction = 'outbound') {
		if (!msg.type) {
			throw new Error('Message missing type');
		}
		
		// Get expected message format from contract
		const schema = this.#getMessageSchema(msg.type, direction);
		if (!schema) {
			throw new Error(`Unknown message type: ${msg.type}`);
		}
		
		// Echo scenario specific validation
		if (msg.type === 'task_request') {
			this.#validateTaskRequest(msg);
		} else if (msg.type === 'task_result') {
			this.#validateTaskResult(msg);
		}
		
		return true;
	}
	
	validateTiming(operation, duration) {
		const limits = {
			'connection': 5000,    // 5 seconds max to establish connection
			'task_response': 1000, // 1 second max for echo response
			'health_check': 100    // 100ms max for health check response
		};
		
		if (duration > limits[operation]) {
			throw new Error(`${operation} exceeded ${limits[operation]}ms limit`);
		}
		
		return true;
	}
	
	#validateTaskRequest(msg) {
		const required = ['id', 'executionId', 'serviceId', 'input'];
		for (const field of required) {
			if (!(field in msg)) {
				throw new Error(`Task request missing required field: ${field}`);
			}
		}
		
		// Echo task specific
		if (!msg.input?.message) {
			throw new Error('Echo task must include message in input');
		}
	}
	
	#validateTaskResult(msg) {
		const required = ['taskId', 'executionId', 'serviceId'];
		for (const field of required) {
			if (!(field in msg)) {
				throw new Error(`Task result missing required field: ${field}`);
			}
		}
		
		// Must have either result or error
		if (!msg.result && !msg.error) {
			throw new Error('Task result must include either result or error');
		}
	}
	
	#getMessageSchema(type, direction) {
		console.log('getMessageSchema', type, direction);
		const messages = this.#contract.components.messages;
		return messages[type]?.payload;
	}
}
