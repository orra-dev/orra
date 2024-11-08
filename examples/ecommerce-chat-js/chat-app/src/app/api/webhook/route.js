/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import { NextResponse } from 'next/server';

export async function POST(req) {
	const body = await req.json();
	
	// Verify webhook signature here if needed
	
	console.log('webhook_data', body);
	
	// Access the global io instance
	if (global.io) {
		global.io.emit('webhook_data', body);
	} else {
		console.warn('Socket.IO not initialized');
	}
	
	return NextResponse.json({ message: 'Webhook received' }, { status: 200 });
}
