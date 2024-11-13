/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

'use client'

import { useEffect, useState } from 'react'
import { PaperAirplaneIcon, TrashIcon } from '@heroicons/react/24/outline'
import { useWebSocket } from '@/app/contexts/WebSocketContext'
import OrchestrationFlow from "@/components/OrchestrationFlow";

const CUSTOMER_ID = 'cust12345'

export default function ChatUI() {
	const [ messages, setMessages ] = useState([])
	const [ inputMessage, setInputMessage ] = useState('')
	const [ orchestrations, setOrchestrations ] = useState({});
	const { socket, isConnected } = useWebSocket();
	
	useEffect(() => {
		if (!socket) return;
		
		socket.on('chat message', (msg) => {
			setMessages(prevMessages => [ ...prevMessages, msg ])
		})
		
		socket.on('orra_err', (data) => {
			const orraMessage = {
				id: Date.now(),
				content: data,
				sender: 'orra_platform',
				type: 'error',
				isJson: false
			}
			setMessages(prevMessages => [ ...prevMessages, orraMessage ])
		})
		
		socket.on('orra_plan', (data) => {
			const orchestrationId = data.id;
			console.log("PLAN:", data.plan)
			setOrchestrations(prev => ({
				...prev,
				[orchestrationId]: {
					plan: data.plan,
					started: true,
					timestamp: new Date(),
					status: 'processing'
				}
			}));
			
			// Also update the triggering message with the orchestrationId
			setMessages(prev => prev.map(msg => {
				if (msg.id === data?.triggeredBy) {
					return { ...msg, orchestrationId };
				}
				return msg;
			}));
		});
		
		socket.on('orchestration_update', (data) => {
			const { id: orchestrationId, status, tasks } = data;
			
			setOrchestrations(prev => ({
				...prev,
				[orchestrationId]: {
					...prev[orchestrationId],
					status,
					tasks,
					lastUpdate: new Date()
				}
			}));
			
			return () => {
				socket.off('chat message');
				socket.off('orra_err');
				socket.off('orra_plan');
				socket.off('orchestration_update');
				socket.off('webhook_data');
			};
		});
		
		socket.on('webhook_data', (data) => {
			const { orchestrationId, status, results, error } = data;
			
			setOrchestrations(prev => {
				// Don't update if we don't have this orchestration
				if (!prev[orchestrationId]) return prev;
				
				return {
					...prev,
					[orchestrationId]: {
						...prev[orchestrationId],
						status,
						webhookData: {
							status,
							results,
							error,
							timestamp: new Date()
						}
					}
				};
			});
		});
		
		// socket.on('orra_plan', (data) => {
		// 	const orraMessage = {
		// 		id: Date.now(),
		// 		content: data,
		// 		sender: 'orra_platform',
		// 		isJson: true
		// 	}
		// 	setMessages(prevMessages => [...prevMessages, orraMessage])
		// })
		
	}, [ socket ])
	
	const sendMessage = (e) => {
		e.preventDefault()
		if (!inputMessage.trim()) return;
		
		const messageId = Date.now();
		const newMessage = {
			id: messageId,
			content: inputMessage,
			sender: 'user',
			customerId: CUSTOMER_ID,
			timestamp: new Date(),
			isJson: false
		}
		
		socket.emit('chat message', {
			...newMessage,
			triggerId: messageId
		});
		setInputMessage('')
	}
	
	const renderMessage = (message) => {
		const isUser = message.sender === 'user';
		const orchestrationData = message.orchestrationId ?
			orchestrations[message.orchestrationId] : null;
		
		return (
			<div key={message.id} className="mb-4">
				<div className={`flex ${isUser ? 'justify-end' : 'justify-start'}`}>
					<div className={`rounded-lg p-3 max-w-xs lg:max-w-md ${
						isUser ? 'bg-blue-500 text-white' : 'bg-white text-gray-800'
					}`}>
						{message.content}
					</div>
				</div>
				
				{orchestrationData && (
					<div className="mt-2 ml-4">
						<OrchestrationFlow
							orchestrationData={orchestrationData}
							onComplete={() => {
								// Optionally handle orchestration completion
								console.log('Orchestration display completed');
							}}
						/>
					</div>
				)}
			</div>
		);
	};
	
	
	return (
		<div className="flex flex-col h-screen">
			<main className="flex-grow bg-gray-100 p-4 overflow-auto">
				<div className="max-w-3xl mx-auto space-y-4">
					{messages.map(renderMessage)}
				</div>
			</main>
			<footer className="bg-white border-t border-gray-200">
				<div className="max-w-3xl mx-auto px-4 py-3 flex items-center space-x-2">
					<form onSubmit={sendMessage} className="flex grow">
            <textarea
	            className="w-full p-2 text-gray-900 border rounded resize-none overflow-hidden"
	            rows="1"
	            placeholder="Type a message..."
	            value={inputMessage}
	            onChange={(e) => setInputMessage(e.target.value)}
            />
						<button
							type="submit"
							className="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-r-md shadow-sm text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
						>
							<PaperAirplaneIcon className="h-5 w-5 mr-2"/>
							Send
						</button>
					</form>
					<button
						onClick={() => {
							setMessages([]);
							setOrchestrations({});
						}}
						className="inline-flex items-center px-4 py-2 bg-red-500 text-white rounded hover:bg-red-600"
					>
						<TrashIcon className="h-5 w-5 mr-2"/>
						Clear
					</button>
				</div>
			</footer>
			<div className="bg-gray-200 p-2 text-center">
        <span className={`inline-flex items-center ${
	        isConnected ? 'text-green-500' : 'text-red-500'
        }`}>
          {isConnected ? '🟢 Connected' : '🔴 Disconnected'}
        </span>
			</div>
		</div>
	)
}
