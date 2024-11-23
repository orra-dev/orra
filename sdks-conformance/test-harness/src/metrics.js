/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

export class MetricsCollector {
	constructor() {
		this.metrics = new Map();
		this.timings = new Map();
	}
	
	startOperation(serviceId, operation) {
		const key = `${serviceId}:${operation}`;
		this.timings.set(key, {
			start: Date.now(),
			operation
		});
	}
	
	endOperation(serviceId, operation) {
		const key = `${serviceId}:${operation}`;
		const timing = this.timings.get(key);
		if (timing) {
			const duration = Date.now() - timing.start;
			this.recordMetric(serviceId, `${operation}_duration`, duration);
			this.timings.delete(key);
			return duration;
		}
	}
	
	recordMetric(serviceId, name, value) {
		const metrics = this.metrics.get(serviceId) || new Map();
		metrics.set(name, value);
		this.metrics.set(serviceId, metrics);
	}
	
	getMetrics(serviceId) {
		return Object.fromEntries(this.metrics.get(serviceId) || new Map());
	}
	
	reset(serviceId) {
		this.metrics.delete(serviceId);
		// Clean up any lingering timings
		for (const [key] of this.timings) {
			if (key.startsWith(serviceId)) {
				this.timings.delete(key);
			}
		}
	}
}

