/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import { ProtocolProxy } from './proxy.js';

const CONTROL_PLANE_URL = process.env.CONTROL_PLANE_URL || 'ws://control-plane:8005';
const PROXY_PORT = parseInt(process.env.PROXY_PORT || '8006', 10);

async function main() {
	const proxy = new ProtocolProxy(CONTROL_PLANE_URL);
	
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

