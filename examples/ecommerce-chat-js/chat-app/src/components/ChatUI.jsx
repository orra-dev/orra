'use client'

import { useEffect, useState } from 'react'
import { PaperAirplaneIcon, TrashIcon } from '@heroicons/react/24/outline'
import JsonView from './JsonView'
import { useWebSocket } from '@/app/contexts/WebSocketContext'

const CUSTOMER_ID = 'cust12345'

export default function ChatUI() {
	const [messages, setMessages] = useState([])
	const [inputMessage, setInputMessage] = useState('')
	const { socket, isConnected } = useWebSocket();
	
	useEffect(() => {
		if (socket) {
			socket.on('chat message', (msg) => {
				setMessages(prevMessages => [...prevMessages, msg])
			})
			
			socket.on('orra_err', (data) => {
				const orraMessage = {
					id: Date.now(),
					content: data,
					sender: 'orra_platform',
					isJson: false
				}
				setMessages(prevMessages => [...prevMessages, orraMessage])
			})
			
			socket.on('webhook_data', (data) => {
				const orraMessage = {
					id: Date.now(),
					content: data,
					sender: 'orra_platform',
					isJson: true
				}
				setMessages(prevMessages => [...prevMessages, orraMessage])
			})
			
			socket.on('orra_plan', (data) => {
				const orraMessage = {
					id: Date.now(),
					content: data,
					sender: 'orra_platform',
					isJson: true
				}
				setMessages(prevMessages => [...prevMessages, orraMessage])
			})
		}
	}, [socket])
	
	const sendMessage = (e) => {
		e.preventDefault()
		if (inputMessage.trim()) {
			const newMessage = {
				id: Date.now(),
				content: inputMessage,
				sender: 'user',
				customerId: CUSTOMER_ID,
				isJson: false
			}
			
			// Check if the message is JSON
			try {
				newMessage.content = JSON.parse(inputMessage);
				newMessage.isJson = true;
			} catch (e) {
				// Not JSON, treat as regular text
			}
			
			socket.emit('chat message', newMessage)
			setInputMessage('')
		}
	}
	
	return (
		<div className="flex flex-col h-screen">
			<main className="flex-grow bg-gray-100 p-4 overflow-auto">
				<div className="max-w-3xl mx-auto space-y-4">
					{messages.map((message) => (
						<div key={message.id} className={`flex ${message.sender === 'user' ? 'justify-end' : 'justify-start'}`}>
							<div
								className={`rounded-lg p-3 ${message.sender === 'user' ? 'bg-blue-500 text-white' : 'bg-white text-gray-800'} ${message.isJson ? 'max-w-full' : 'max-w-xs'}`}>
								{message.isJson ? (
									<JsonView data={message.content}/>
								) : (
									message.content
								)}
							</div>
						</div>
					))}
				</div>
			</main>
			<footer className="bg-white border-t border-gray-200">
				<div className="max-w-3xl mx-auto px-4 py-3 flex items-center space-x-2">
					<form onSubmit={sendMessage} className="flex grow">
						{/*<input*/}
						{/*	type=""*/}
						{/*	className="text-gray-900 flex-grow rounded-l-md border-2 px-4 py-2 border-gray-300 focus:ring-2 focus:ring-blue-500 focus:border-blue-500"*/}
						{/*	placeholder="Type a message..."*/}
						{/*	value={inputMessage}*/}
						{/*	onChange={(e) => setInputMessage(e.target.value)}*/}
						{/*/>*/}
						<textarea
							className="w-full p-2 text-gray-900 border rounded resize-none overflow-hidden"
							rows="1"
							placeholder="Type a message..."
							value={inputMessage}
							onChange={(e) => setInputMessage(e.target.value)}
						></textarea>
						<button
							type="submit"
							className="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-r-md shadow-sm text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
						>
							<PaperAirplaneIcon className="h-5 w-5 mr-2"/>
							Send
						</button>
					</form>
					<button
						onClick={() => setMessages([])}
						className="inline-flex items-center px-4 py-2 bg-red-500 text-white rounded hover:bg-red-600"
					>
						<TrashIcon className="h-5 w-5 mr-2"/>
						Clear
					</button>
				</div>
			</footer>
			<div className="bg-gray-200 p-2 text-center text-green-500">
				Connection status: {isConnected ? 'Connected' : 'Disconnected'}
			</div>
		</div>
	)
}
