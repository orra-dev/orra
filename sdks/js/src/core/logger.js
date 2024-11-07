/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import pino from 'pino';

export class OrraLogger {
	#logger;
	#baseConfig;
	
	constructor(opts = {}) {
		this.#baseConfig = {
			level: process.env.ORRA_LOG_LEVEL || 'error',
			enabled: process.env.ORRA_LOGGING !== 'false',
			transport: process.env.ORRA_LOG_PRETTY === 'true' ? {
				target: 'pino-pretty',
				options: { colorize: true }
			} : undefined
		};
		
		this.reconfigure(opts);
	}
	
	reconfigure(opts = {}) {
		this.#logger = pino({
			...this.#baseConfig,
			base: {
				sdk: 'orra',
				serviceId: opts.serviceId,
				serviceVersion: opts.serviceVersion
			}
		});
	}
	
	error(msg, meta = {}) { this.#logger.error(meta, msg); }
	warn(msg, meta = {}) { this.#logger.warn(meta, msg); }
	info(msg, meta = {}) { this.#logger.info(meta, msg); }
	debug(msg, meta = {}) { this.#logger.debug(meta, msg); }
	trace(msg, meta = {}) { this.#logger.trace(meta, msg); }
}
