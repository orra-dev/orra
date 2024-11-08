/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

'use client';

import React, { createContext, useContext, useEffect, useState } from 'react';
import io from 'socket.io-client';

const WebSocketContext = createContext({ socket: null, isConnected: false });

export function WebSocketProvider({ children }) {
	const [socket, setSocket] = useState(null);
	const [isConnected, setIsConnected] = useState(false);
	
	useEffect(() => {
		const newSocket = io();
		
		newSocket.on('connect', () => {
			setIsConnected(true);
		});
		
		newSocket.on('disconnect', () => {
			setIsConnected(false);
		});
		
		setSocket(newSocket);
		
		return () => {
			newSocket.close();
		};
	}, []);
	
	return (
		<WebSocketContext.Provider value={{ socket, isConnected }}>
			{children}
		</WebSocketContext.Provider>
	);
}

export const useWebSocket = () => useContext(WebSocketContext);
