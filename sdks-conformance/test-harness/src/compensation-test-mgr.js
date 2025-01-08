/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

export class CompensationTestManager {
	constructor(webhookResults) {
		this.webhookResults = webhookResults;
		this.successfulTaskResult = null;
		this.state = 'initial';
	}
	
	handleTaskResult(testId, result) {
		switch (this.state) {
			case 'initial':
				if (result.result && !result.error) {
					this.successfulTaskResult = result;
					this.state = 'success_recorded';
					this.recordTestEvent(testId, 'first_task_completed');
				}
				break;
			
			case 'success_recorded':
				if (result.error) {
					this.state = 'failure_recorded';
					this.recordTestEvent(testId, 'second_task_failed');
				}
				break;
			
			case 'compensation_started':
				if (result?.result?.status === 'completed') {
					this.state = 'completed';
					this.recordTestEvent(testId, 'compensation_completed');
				}
				break;
		}
		
		return this.state;
	}
	
	recordTestEvent(testId, eventType) {
		const testResult = this.webhookResults.get(testId);
		if (testResult) {
			testResult.results.push({
				type: eventType,
				timestamp: new Date().toISOString()
			});
			if (eventType === 'compensation_completed') {
				testResult.status = 'completed';
			}
			this.webhookResults.set(testId, testResult);
		}
	}
	
	getSuccessfulTaskResult() {
		return this.successfulTaskResult;
	}
	
	activateCompensationStarted(){
		this.state = 'compensation_started';
	}
}

