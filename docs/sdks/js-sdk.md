# Orra JS SDK Documentation

The JS SDK for Orra lets you transform your AI agents, tools as services and services into reliable, production-ready Node.js components.

## Installation

First, [install the Orra CLI](../cli.md) .

Then, install the latest version of the SDK.
```bash
# Install the SDK
npm install -S @orra.dev/sdk
```

## Quick Integration Example

The Orra SDK is designed to wrap your existing logic with minimal changes. Here's a simple example showing how to integrate an existing customer chat service:

```javascript
import { initService } from '@orra.dev/sdk';
import { myService } from './existing-service';  // Your existing logic

// Initialize the Orra client
const customerChatSvc = initService({
	name: 'customer-chat-service',
	orraUrl: process.env.ORRA_URL,
	orraKey: process.env.ORRA_API_KEY
});

// Register your service
await customerChatSvc.register({
	description: 'Handles customer chat interactions',
	schema: {
		input: {
			type: 'object',
			properties: {
				customerId: { type: 'string' },
				message: { type: 'string' }
			},
			required: [ 'customerId', 'message' ]
		},
		output: {
			type: 'object',
			properties: {
				response: { type: 'string' }
			}
		}
	}
});

// Wrap your existing business logic
customerChatSvc.start(async (task) => {
	try {
		const { customerId, message } = task.input;
		
		// Use your existing service function
		// Your function handles its own retries and error recovery
		// and Orra reacts accordingly.
		const response = await myService(customerId, message);
		
		return { response };
	} catch (error) {
		// Once you determine the task should fail, throw the error.
		// Orra will handle failure propagation to the control plane.
		throw error;
	}
});
```

## Understanding the SDK

The Orra SDK follows patterns similar to serverless functions or job processors, making it familiar for AI Engineers. Your services become event-driven handlers that:

1. Register capabilities with Orra's Plan Engine (what they can do)
2. Process tasks when called (actual execution)
3. Return results for orchestration

### Key Concepts

- **Services and Tasks as Services vs Agents**: Both use the same SDK but are registered differently
    - Services and Tasks as Services: Stateless, function-like handlers (e.g., data processors, notification services, etc...)
    - Agents: Stateless or stateful, sometimes long-running processes, see [What is an AI Agent](../what-is-agent.md)

- **Schema Definition**: Similar to OpenAPI/GraphQL schemas, defines inputs/outputs
- **Handler Functions**: Like serverless functions, process single tasks
- **Health Monitoring**: Automatic health checking and reporting
- **Service Identity**: Maintained across restarts and updates

## Detailed Integration Guide

### 1. Service/Agent Registration

Services and Agents names must also follow these rules:
- They are limited to 63 characters, and at least 3 chars in length
- Consist of lowercase alphanumeric characters
- Can include hyphens (-) and dots (.)
- Must start and end with an alphanumeric character

Register your service with its capabilities:

```javascript
const client = initService({
   name: 'service-name',
   orraUrl: process.env.ORRA_URL,
   orraKey: process.env.ORRA_API_KEY
});

// For stateless services
await client.register({
   description: 'What this service does',
   schema: {
      input: {
         type: 'object',
         properties: {
            // Define expected inputs
         }
      },
      output: {
         type: 'object',
         properties: {
            // Define expected outputs
         }
      }
   }
});

// For AI agents
const client = initAgent({
   name: 'agent-name',
   orraUrl: process.env.ORRA_URL,
   orraKey: process.env.ORRA_API_KEY
});

await client.register({
   description: 'What this agent does',
   schema: {
      // Same schema structure as services
   }
});
```

### 2. Task Handler Implementation

Implement your task handler to process requests:

```javascript
service.start(async (task) => {
  try {
    // 1. Access task information
    const { input, executionId } = task;
    
    // 2. Your existing business logic, may include its own retry/recovery if available (otherwise Orra deals with this) 
    const result = await yourExistingFunction(input);
    
    // 3. Return results
    return result;
  } catch (error) {
    // After your error handling is complete, let Orra know about permanent failures
    throw error;
  }
});
```

### 3. Reverts powered by compensations

Marking a service or agent as **revertible** enables the previous task result to be compensated by that component in case of upstream failures.

A revert may succeed **completely**, **partially** or simply **fail**. They are run after an action's failure.

Checkout the [Compensations](../compensations.md) doc for full explanation.

```javascript
// 1. Register the service as revertible
await client.register({
   description: 'What this service does',
   // ...
   revertible: true
});

// 2. Add the revert handler, which accepts the original task the actual task result for an orchestration
service.onRevert(async (task, result) => {
  console.log('Reverting for task:', task.id);
  console.log('Reverting inventory product hold from:', result?.hold, 'to:', false);
  // If this errors, Orra will try to re-compensate upto 10 times.
})
```

## Advanced Features

### Persistence Configuration

Orra maintains service/agent identity across restarts using persistence. This is crucial for:
- Maintaining service/agent history
- Ensuring consistent service/agent identification
- Supporting service/agent upgrades

```javascript
const service = initService({
   name: 'a-service',
   orraUrl: process.env.ORRA_URL,
   orraKey: process.env.ORRA_API_KEY,
   persistenceOpts: {
      // 1. File-based persistence (default)
      method: 'file',
      filePath: './custom/path/service-key.json',

      // 2. Custom persistence (e.g., database)
      method: 'custom',
      customSave: async (id) => {
         await db.services.save(id);
      },
      customLoad: async () => {
         return await db.services.load();
      }
   }
});
```

## Best Practices

1. **Error Handling**
    - Implement comprehensive error handling in your business logic
    - Use retries for transient failures

2. **Schema Design**
    - Be specific about input/output types
    - Include comprehensive descriptions
    - Keep schemas focused and minimal

3. **Service Design**
    - Keep services focused on specific capabilities
    - Design for idempotency
    - Include proper logging for debugging

## Example: Converting Existing Code

Here's how to convert an existing AI service to use Orra:

### Before (Traditional Express API)
```javascript
import express from 'express';
import { analyzeImage } from './ai-agent';

const app = express();

app.post('/analyze', async (req, res) => {
  const { imageUrl } = req.body;
  try {
    const analysis = await analyzeImage(imageUrl);
    res.json(analysis);
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

app.listen(3000);
```

### After (Orra Integration)
```javascript
import { initAgent } from '@orra.dev/sdk';
import { analyzeImage } from './ai-agent';  // Reuse existing logic

const imageAgent = initAgent({
  name: 'image-analysis-agent',
  orraUrl: process.env.ORRA_URL,
  orraKey: process.env.ORRA_API_KEY
});

await imageAgent.register({
  description: 'Analyzes any image using AI',
  schema: {
    input: {
      type: 'object',
      properties: {
        imageUrl: { 
          type: 'string',
          format: 'uri'
        }
      },
      required: ['imageUrl']
    },
    output: {
      type: 'object',
      properties: {
        objects: { type: 'array' },
        labels: { type: 'array' },
        confidence: { type: 'number' }
      }
    }
  }
});

// Reuse your existing analysis function
imageAgent.start(async (task) => {
  try {
    const { imageUrl } = task.input;
    // Your function handles its own retries
    return await analyzeImage(imageUrl);
  } catch (error) {
    // After your error handling is complete, let Orra know about the failure
    throw error;
  }
});

// Graceful shutdown
process.on('SIGTERM', async () => {
	console.log('SIGTERM received, shutting down gracefully');
	imageAgent.shutdown();
	process.exit(0);
});
```

## Next Steps

1. Review the example projects for more integration patterns
2. Join our community for support and updates
3. Check out the action orchestration guide to start using your services

Need help? Contact the Orra team or open an issue on GitHub.
