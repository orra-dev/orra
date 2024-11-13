/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import { NextResponse } from 'next/server';

export async function POST(req) {
	const data = await req.json();
	
	// Verify webhook signature here if needed
	
	console.log('webhook_data', data);
	
	// Access the global io instance
	if (global.io) {
		global.io.emit('webhook_data', {
			orchestrationId: data.orchestrationId,
			status: data.status,
			results: data.results,
			error: data.error
		});
	} else {
		console.warn('Socket.IO not initialized');
	}
	
	return NextResponse.json({ message: 'Webhook received' }, { status: 200 });
}
