/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import { ProtocolProxy } from './proxy.js';
import { join } from "path";
import { fileURLToPath } from "url";
import * as path from "node:path";

const CONTROL_PLANE_URL = process?.env?.CONTROL_PLANE_URL || 'http://control-plane:8005';
const PROXY_PORT = parseInt(process?.env?.PROXY_PORT || '8006', 10);

console.log('process?.env?.SDK_CONTRACT_PATH', process?.env?.SDK_CONTRACT_PATH)

const SDK_CONTRACT_PATH = process?.env?.SDK_CONTRACT_PATH ||
	join(getDirName(import.meta.url), "../../contracts/sdk.yaml")

async function main() {
	const proxy = new ProtocolProxy(CONTROL_PLANE_URL, SDK_CONTRACT_PATH);
	
	// Handle shutdown gracefully
	process.on('SIGTERM', async () => {
		console.log('Shutting down proxy...');
		await proxy.stop();
		process.exit(0);
	});
	
	try {
		await proxy.start(PROXY_PORT);
		console.log(`Protocol validation proxy running on port ${PROXY_PORT}`);
		console.log(`Proxying to control plane at ${CONTROL_PLANE_URL}`);
	} catch (error) {
		console.error('Failed to start proxy:', error);
		process.exit(1);
	}
}

main().catch(console.error);

function getDirName(moduleUrl) {
	const filename = fileURLToPath(moduleUrl);
	return path.dirname(filename);
}
