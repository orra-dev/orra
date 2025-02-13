/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import { useState, useEffect } from 'react';
import { Activity, Bot, Cpu, CheckCircle2, XCircle, MapPin, Calendar } from 'lucide-react';

const OrchestrationFlow = ({ orchestrationData }) => {
	const [analysisShown, setAnalysisShown] = useState(false);
	const [currentTasks, setCurrentTasks] = useState([]);
	const [resultShown, setResultShown] = useState(false);
	
	useEffect(() => {
		if (!orchestrationData?.started || analysisShown) return;
		
		const timer = setTimeout(() => {
			setAnalysisShown(true);
		}, 800);
		
		return () => clearTimeout(timer);
	}, [orchestrationData?.started, analysisShown]);
	
	useEffect(() => {
		if (!orchestrationData?.plan || !analysisShown) return;
		
		const { tasks, parallel_groups } = orchestrationData.plan;
		
		const getTaskDescription = (taskName) => {
			if (taskName.toLowerCase().includes('customer')) {
				return `Processing customer details ...`;
			}
			if (taskName.toLowerCase().includes('inventory')) {
				return `Processing product details ...`;
			}
			if (taskName.toLowerCase().includes('delivery')) {
				return 'Performing deep analysis on customer address and product details to compute delivery...';
			}
			return 'Processing request...';
		};
		
		parallel_groups.forEach((group, groupIndex) => {
			const groupTasks = group
				.map(taskId => {
					const task = tasks.find(t => t.id === taskId);
					if (!task || task.id === 'task0') return null;
					
					return {
						id: taskId,
						type: task['service_name']?.toLowerCase().includes('agent') ? 'agent' : 'service',
						name: task['service_name'] || task.service,
						description: getTaskDescription(task['service_name']),
						status: 'processing'
					};
				})
				.filter(Boolean);
			
			setTimeout(() => {
				setCurrentTasks(prev => [...prev, ...groupTasks]);
			}, (groupIndex + 1) * 1000);
		});
	}, [orchestrationData?.plan, analysisShown]);
	
	useEffect(() => {
		if (!orchestrationData?.webhookData || resultShown) return;
		
		setCurrentTasks(prev =>
			prev.map(task => ({ ...task, status: 'completed' }))
		);
		
		setTimeout(() => {
			setResultShown(true);
		}, 1000);
	}, [orchestrationData?.webhookData, resultShown]);
	
	const renderTaskMessage = (task) => (
		<div key={task.id} className="flex items-start space-x-3 py-2">
			{task.type === 'agent' ?
				<Bot className="h-5 w-5 text-purple-500" /> :
				<Cpu className="h-5 w-5 text-yellow-500" />
			}
			<div className="flex-1">
				<div className="font-bold text-gray-700">{task.name}</div>
				<div className="text-gray-700">{task.description}</div>
				<div className={`text-sm mt-1 ${
					task.status === 'completed' ? 'text-green-500' : 'text-yellow-500 animate-pulse'
				}`}>
					{task.status === 'completed' ? 'Completed' : 'Processing'}
				</div>
			</div>
		</div>
	);
	
	const renderResult = () => {
		if (!orchestrationData?.webhookData) return null;
		const { status, results, error } = orchestrationData.webhookData;
		
		if (status === 'failed') {
			return (
				<div className="flex items-start space-x-3 py-2">
					<XCircle className="h-5 w-5 text-red-500" />
					<div className="text-red-600">
						Unable to complete request: {error}
					</div>
				</div>
			);
		}
		
		if (status === 'completed' && results) {
			try {
				const lastResult = results[results.length - 1];
				const data = (typeof lastResult === 'string' ? JSON.parse(lastResult) : lastResult)?.response;
				
				if (!data?.delivery_estimates) return null;
				
				const bestCase = data.delivery_estimates.estimates.find(e => e.scenario === "Best case");
				const worstCase = data.delivery_estimates.estimates.find(e => e.scenario === "Worst case");
				
				return (
					<div className="space-y-4 py-2">
						<div className="flex items-start space-x-3">
							<CheckCircle2 className="h-5 w-5 text-green-500" />
							<div className="flex-1 space-y-4">
								<div className="space-y-2">
									<div className="flex items-center space-x-2">
										<Calendar className="h-4 w-4" />
										<h3 className="font-bold text-gray-700">Delivery Window</h3>
									</div>
									<div className="pl-6 text-gray-600 whitespace-pre-line">
										📅 Earliest: {formatDateTime(bestCase.estimated_arrival_time)}
										<br/>
										📅 Latest: {formatDateTime(worstCase.estimated_arrival_time)}
									</div>
								</div>
								
								<div className="space-y-2">
									<div className="flex items-center space-x-2">
										<MapPin className="h-4 w-4" />
										<h3 className="font-medium text-gray-700">Journey Details</h3>
									</div>
									<div className="pl-6 text-gray-600 whitespace-pre-line">
										📍 Distance: {data.delivery_estimates.route_summary.total_distance_km}km
										⏱️ Duration: {bestCase.estimated_duration_hours} - {worstCase.estimated_duration_hours} hours
										<br/>
										{getConfidenceEmoji(bestCase.confidence_level)} Confidence: {capitalize(bestCase.confidence_level)}
									</div>
								</div>
							</div>
						</div>
					</div>
				);
			} catch (e) {
				return (
					<div className="flex items-start space-x-3 py-2">
						<XCircle className="h-5 w-5 text-red-500" />
						<div className="text-red-600">
							Sorry, I couldn&#39;t process the delivery information
						</div>
					</div>
				);
			}
		}
	};
	
	if (!orchestrationData) return null;
	
	return (
		<div className="space-y-2 bg-white rounded-lg shadow-sm p-4">
			{analysisShown && (
				<div className="flex items-start space-x-3 py-2">
					<Activity className="h-5 w-5 text-blue-500 animate-pulse" />
					<div className="text-gray-700">
						🔍 Analyzing your request...
					</div>
				</div>
			)}
			
			{currentTasks.map(renderTaskMessage)}
			
			{resultShown && renderResult()}
		</div>
	);
};

const formatDateTime = (dateStr) => {
	return new Date(dateStr).toLocaleString('en-US', {
		weekday: 'short',
		month: 'short',
		day: 'numeric',
		hour: 'numeric',
		minute: 'numeric',
		hour12: true
	});
};

const getConfidenceEmoji = (level) => {
	switch (level.toLowerCase()) {
		case 'high': return '🎯';
		case 'moderate': return '👍';
		default: return '📊';
	}
};

const capitalize = (str) => {
	return str.charAt(0).toUpperCase() + str.slice(1).toLowerCase();
};

export default OrchestrationFlow;
