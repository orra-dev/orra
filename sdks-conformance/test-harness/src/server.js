/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import { ConformanceServer } from './conformance-server.js';
import { join } from "path";
import { fileURLToPath } from "url";
import * as path from "node:path";

const PLAN_ENGINE_URL = process?.env?.PLAN_ENGINE_URL || 'http://plan-engine:8005';
const SDK_TEST_HARNESS_PORT = parseInt(process?.env?.SDK_TEST_HARNESS_PORT || '8006', 10);

console.log('process?.env?.SDK_CONTRACT_PATH', process?.env?.SDK_CONTRACT_PATH)

const SDK_CONTRACT_PATH = process?.env?.SDK_CONTRACT_PATH ||
	join(getDirName(import.meta.url), "../../contracts/sdk.yaml")

async function main() {
	const conformanceServer = new ConformanceServer(PLAN_ENGINE_URL, SDK_CONTRACT_PATH);
	
	// Handle shutdown gracefully
	process.on('SIGTERM', async () => {
		console.log('Shutting down conformanceServer...');
		await conformanceServer.stop();
		process.exit(0);
	});
	
	try {
		await conformanceServer.start(SDK_TEST_HARNESS_PORT);
		console.log(`Protocol validation proxy running on port ${SDK_TEST_HARNESS_PORT}`);
		console.log(`Proxying to plan engine at ${PLAN_ENGINE_URL}`);
	} catch (error) {
		console.error('Failed to start conformanceServer:', error);
		process.exit(1);
	}
}

main().catch(console.error);

function getDirName(moduleUrl) {
	const filename = fileURLToPath(moduleUrl);
	return path.dirname(filename);
}
