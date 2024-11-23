/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

export function createWebhookHandler(resultStore) {
	return {
		handleWebhook: async (req, res) => {
			if (req.method !== 'POST') {
				res.statusCode = 405;
				res.end(JSON.stringify({ error: 'Method not allowed' }));
				return;
			}
			
			// Read webhook body
			let body = '';
			for await (const chunk of req) {
				body += chunk;
			}
			
			try {
				const webhookData = JSON.parse(body);
				
				// Store result for verification
				resultStore.set(webhookData.orchestrationId, {
					status: webhookData.status,
					results: webhookData.results,
					error: webhookData.error,
					timestamp: new Date()
				});
				
				// Return success
				res.statusCode = 200;
				res.end(JSON.stringify({ status: 'ok' }));
				
			} catch (error) {
				res.statusCode = 400;
				res.end(JSON.stringify({ error: 'Invalid webhook data' }));
			}
		}
	};
}
