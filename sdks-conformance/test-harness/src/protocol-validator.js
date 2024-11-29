/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import { parse } from 'yaml';
import { readFileSync } from 'fs';
import assert from 'node:assert';

const CONTRACT_MESSAGE_TYPE_MAPPING = {
	'task_request': 'TaskRequest',
	'task_result': 'TaskResult',
	'task_status': 'TaskStatus',
	'ping': 'Ping',
	'pong': 'Pong',
	'ACK': 'Ack'
}

export class ProtocolValidator {
	contract;
	
	constructor(sdkContractPath) {
		const contractYaml = readFileSync(sdkContractPath, 'utf8');
		this.contract = parse(contractYaml);
	}
	
	validateConnection(params) {
		const { serviceId, apiKey } = params;
		
		assert(serviceId, 'serviceId is required');
		assert(apiKey, 'apiKey is required');
		assert(serviceId.startsWith('s_'), 'Invalid serviceId format');

		return this.validateApiKey(apiKey);
	}
	
	validateApiKey(apiKey) {
		assert(apiKey.startsWith('sk-orra-'), 'Invalid apiKey format');
		return true
	}
	
	validateMessage(msg, direction = 'sdk-outbound') {
		if (direction === 'sdk-outbound'){
			msg = msg.payload
		}
		
		assert(msg.type, 'Message missing type');
		assert(CONTRACT_MESSAGE_TYPE_MAPPING[msg.type], `Unknown message type: ${msg.type}`);
		
		const schema = this.getMessageSchema(CONTRACT_MESSAGE_TYPE_MAPPING[msg.type], direction);
		assert(schema, `Schema is missing for message type: ${msg.type}`);
		
		const methodName = `validate${CONTRACT_MESSAGE_TYPE_MAPPING[msg.type]}`;
		assert(typeof this[methodName] === 'function', `No validation method found for message type: ${msg.type}`);
		this[methodName](msg, schema);
		
		return true;
	}
	
	validateTiming(operation, duration) {
		const limits = {
			'connection': 5000,    // 5 seconds max to establish connection
			'task_response': 1000, // 1 second max for echo response
			'health_check': 100    // 100ms max for health check response
		};
		
		assert(duration <= limits[operation], `${operation} exceeded ${limits[operation]}ms limit`);
		return true;
	}
	
	validateAck(msg, schema) {
		for (const field of schema.required) {
			assert(field in msg, `Acknowledgement message missing required field: ${field}`);
		}
		assert(schema.properties.type.enum[0] === msg.type, 'Type incorrect for an acknowledgement message');
	}
	
	validatePing(msg, schema) {
		for (const field of schema.required) {
			assert(field in msg, `Ping request missing required field: ${field}`);
		}
		assert(schema.properties.type.enum[0] === msg.type, 'Type incorrect for ping message');
	}
	
	validatePong(msg, schema) {
		for (const field of schema.required) {
			assert(field in msg, `Pong request missing required field: ${field}`);
		}
		assert(schema.properties.type.enum[0] === msg.type, 'Type incorrect for pong message');
	}
	
	validateTaskRequest(msg, schema) {
		for (const field of schema.required) {
			assert(field in msg, `Task request missing required field: ${field}`);
		}
	}
	
	validateTaskStatus(msg, schema) {
		for (const field of schema.required) {
			assert(field in msg, `Task status missing required field: ${field}`);
		}
		assert(schema.properties.type.enum[0] === msg.type, 'Type incorrect for task status message');
		assert(schema.properties.status.enum[0] === 'in_progress', 'Status incorrect for task status message');
	}
	
	validateTaskResult(msg, schema) {
		for (const field of schema.required) {
			assert(field in msg, `Task result missing required field: ${field}`);
		}
		
		// Must have either result or error
		assert(msg.result || msg.error, 'Task result must include either result or error');
	}
	
	getMessageSchema(type, direction) {
		console.log('getMessageSchema', type, direction);
		const messages = this.contract.components.messages;
		return messages[type]?.payload;
	}
}
