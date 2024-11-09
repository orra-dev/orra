/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import React, { useState } from 'react';

const JsonView = ({ data }) => {
	const [expanded, setExpanded] = useState({});
	
	const toggleExpand = (key) => {
		setExpanded(prev => ({ ...prev, [key]: !prev[key] }));
	};
	
	const renderValue = (value, key, path) => {
		if (typeof value === 'object' && value !== null) {
			return renderObject(value, key, path);
		}
		return (
			<div className="json-pair">
				<span className="json-key">{key}: </span>
				<span className="json-value">{JSON.stringify(value)}</span>
			</div>
		);
	};
	
	const renderObject = (obj, key, path = '') => {
		obj = typeof obj === 'string' ? JSON.parse(obj) : obj;
		const currentPath = path ? `${path}.${key}` : key;
		const isExpanded = expanded[currentPath];
		const isArray = Array.isArray(obj);
		const isEmpty = Object.keys(obj).length === 0;
		
		return (
			<div key={currentPath} className="json-object">
        <span
	        className="json-toggle"
	        onClick={() => toggleExpand(currentPath)}
        >
          {isExpanded ? '▼' : '▶'}
	        {key !== 'root' && <span className="json-key">{key}: </span>}
	        {isArray ? '[' : '{'}
        </span>
				{isExpanded && !isEmpty ? (
					<div className="json-nested">
						{Object.entries(obj).map(([k, v]) => renderValue(v, k, currentPath))}
					</div>
				) : null}
				{isExpanded ? (
					<span>{isArray ? ']' : '}'}</span>
				) : (
					<span> {isEmpty ? (isArray ? '[]' : '{}') : '...'}</span>
				)}
			</div>
		);
	};
	
	return (
		<div className="json-view">
			{renderObject(data, 'root')}
		</div>
	);
};

export default JsonView;
