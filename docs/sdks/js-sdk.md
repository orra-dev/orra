# Orra JS SDK Documentation

The JS SDK for orra lets you transform your AI agents, tools as services and services into reliable, production-ready Node.js components.

## Installation

First, [install the orra CLI](../cli.md) .

Then, install the latest version of the SDK.
```bash
# Install the SDK
npm install -S @orra.dev/sdk
```

## Quick Integration Example

The orra SDK is designed to wrap your existing logic with minimal changes. Here's a simple example showing how to integrate an existing customer chat service:

```javascript
import { initService } from '@orra.dev/sdk';
import { myService } from './existing-service';  // Your existing logic

// Initialize the orra client
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
		// and orra reacts accordingly.
		const response = await myService(customerId, message);
		
		return { response };
	} catch (error) {
		// Once you determine the task should fail, throw the error.
		// orra will handle failure propagation to the Plan Engine.
		throw error;
	}
});
```

## Understanding the SDK

The orra SDK follows patterns similar to serverless functions or job processors, making it familiar for AI Engineers. Your services become event-driven handlers that:

1. Register capabilities with orra's Plan Engine (what they can do)
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
    
    // 2. Your existing business logic, may include its own retry/recovery if available (otherwise orra deals with this) 
    const result = await yourExistingFunction(input);
    
    // 3. Return results
    return result;
  } catch (error) {
    // After your error handling is complete, let orra know about permanent failures
    throw error;
  }
});
```

### 3. Aborting Tasks

You can abort a task's execution when you need to stop processing but don't want to trigger a normal failure. This is useful for business logic conditions where it makes sense to halt the orchestration with a specific message:

```javascript
service.start(async (task) => {
  try {
    // Check inventory availability
    const inventory = await checkInventory(task.input.productId);
    
    if (inventory.available < task.input.quantity) {
      // Abort with detailed payload - will stop execution
      return task.abort({
        reason: "INSUFFICIENT_INVENTORY",
        available: inventory.available,
        requested: task.input.quantity
      });
    }
    
    // Normal processing continues if not aborted
    const reservationId = await createReservation(task.input);
    return { success: true, reservationId };
  } catch (error) {
    throw error;
  }
});
```

When a task is aborted:
- The orchestration stops execution
- The abort payload is preserved
- If any previously executed services are revertible, their compensation will be triggered
- The abort information is available in the compensation context

### 4. Reverts powered by compensations

Marking a service or agent as **revertible** enables the previous task result to be compensated by that component in case of upstream failures or aborts.

A revert may succeed **completely**, **partially** or simply **fail**. They are run after an action's failure.

Checkout the [Compensations](../compensations.md) doc for full explanation.

```javascript
// 1. Register the service as revertible
await client.register({
   description: 'What this service does',
   // ...
   revertible: true
});

// 2. Add the revert handler, which accepts the original task, the actual task result, and the compensation context
service.onRevert(async (task, result, context) => {
  console.log('Reverting for task:', task.id);
  console.log('Reverting inventory product hold from:', result?.hold, 'to:', false);
  
  // Access compensation context information
  if (context) {
    // Why the compensation was triggered during an orchestration: "failed", "aborted", etc.
    console.log('Compensation reason:', context.reason);
    
    // ID of the parent orchestration
    console.log('Orchestration ID:', context.orchestrationId);
    
    // When the compensation was initiated
    console.log('Timestamp:', context.timestamp);
    
    // Access abort payload if task was aborted
    if (context.reason === 'aborted' && context.payload) {
      console.log('Abort reason:', context.payload.reason);
      console.log('Available inventory:', context.payload.available);
      console.log('Requested quantity:', context.payload.requested);
    }
  }
  
  // Perform compensation logic
  await releaseInventoryHold(result.reservationId);
  
  // Return compensation result
  return {
    status: 'completed'
  };
  
  // For partial compensation:
  // return {
  //   status: 'partial',
  //   partial: {
  //     completed: ['inventory_hold'],
  //     remaining: ['notification']
  //   }
  // };
});
```

#### Compensation Context

The compensation context provides critical information about why a compensation was triggered:

| Property | Description                                                                                            |
|----------|--------------------------------------------------------------------------------------------------------|
| `reason` | Why the compensation was triggered during an orchestration (e.g., `"failed"`, `"aborted"`)             |
| `orchestrationId` | The ID of the parent orchestration                                                                     |
| `timestamp` | When the compensation was initiated                                                                    |
| `payload` | **Optional**: additional data related to the compensation (may contain aborted or failed payload) |

This context helps you implement more intelligent compensation logic based on the specific reason for the rollback.

## Advanced Features

### Progress Updates

For long-running tasks, you can send interim progress updates to the Plan Engine. This allows monitoring task execution in real-time and provides valuable information for debugging and audit logs.

The update is any object with properties that make sense for the task underway.

```javascript
service.start(async (task) => {
  try {
    // 1. Begin processing
    await task.pushUpdate({
      progress: 20,
      status: "processing",
      message: "Starting data analysis"
    });
    
    // 2. Continue with more steps
    await someFunction();
    await task.pushUpdate({
      progress: 50,
      status: "processing",
      message: "Processing halfway complete"
    });
    
    // 3. Almost done
    await finalSteps();
    await task.pushUpdate({
      progress: 90,
      status: "processing",
      message: "Finalizing results"
    });
    
    // 4. Return final result
    return { success: true, results: [...] };
  } catch (error) {
    // Handle errors
    throw error;
  }
});
```

#### Benefits of Progress Updates

- **Visibility**: Track execution of long-running tasks in real-time
- **Debugging**: Identify exactly where tasks slow down or fail
- **Audit Trail**: Maintain a complete history of task execution
- **User Experience**: Forward progress information to end-users

#### Viewing Progress Updates

Use the orra CLI to view progress updates:

```bash
# View summarized progress updates
orra inspect -d <orchestration-id> --updates

# View all detailed progress updates
orra inspect -d <orchestration-id> --long-updates
```

#### Best Practices

- Send updates at meaningful milestones (not too frequent)
- Include percentage completion when possible
- Keep messages concise and informative
- Use consistent status terminology

### Persistence Configuration

orra's Plan Engine maintains service/agent identity across restarts using persistence. This is crucial for:
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
    - Use `task.abort()` for business-logic conditions that should stop execution

2. **Schema Design**
    - Be specific about input/output types
    - Include comprehensive descriptions
    - Keep schemas focused and minimal

3. **Service Design**
    - Keep services focused on specific capabilities
    - Design for idempotency
    - Include proper logging for debugging

4. **Compensation Design**
    - Make compensation logic robust with its own error handling
    - Use the compensation context to implement context-aware reverts
    - Test compensation scenarios thoroughly

## Example: Converting Existing Code

Here's how to convert an existing AI service to use orra:

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

### After (orra Integration)
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
    
    // Check if the image is accessible
    const isAccessible = await checkImageAccess(imageUrl);
    if (!isAccessible) {
      return task.abort({
        reason: "IMAGE_NOT_ACCESSIBLE",
        url: imageUrl
      });
    }
    
    // Your function handles its own retries
    return await analyzeImage(imageUrl);
  } catch (error) {
    // After your error handling is complete, let orra know about the failure
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

## Complete Example: Revertible Service with Abort and Context

Here's a complete example of an inventory service that demonstrates abort functionality and context-aware compensation:

```javascript
import { initService } from '@orra.dev/sdk';
import { inventoryDb } from './inventory-db';

const inventoryService = initService({
  name: 'inventory-service',
  orraUrl: process.env.ORRA_URL,
  orraKey: process.env.ORRA_API_KEY
});

await inventoryService.register({
  description: 'Handles product inventory reservations',
  revertible: true,  // Enable compensations
  schema: {
    input: {
      type: 'object',
      properties: {
        orderId: { type: 'string' },
        productId: { type: 'string' },
        quantity: { type: 'number' }
      },
      required: ['orderId', 'productId', 'quantity']
    },
    output: {
      type: 'object',
      properties: {
        success: { type: 'boolean' },
        reservationId: { type: 'string' }
      }
    }
  }
});

// Main task handler
inventoryService.start(async (task) => {
  try {
    const { orderId, productId, quantity } = task.input;
    
    // Send progress update
    await task.pushUpdate({
      progress: 25,
      status: "checking_inventory",
      message: "Checking product availability"
    });
    
    // Check inventory
    const available = await inventoryDb.checkAvailability(productId);
    
    if (available < quantity) {
      // Abort the task with detailed information
      return task.abort({
        reason: "INSUFFICIENT_INVENTORY",
        available: available,
        requested: quantity,
        productId: productId
      });
    }
    
    // Update progress
    await task.pushUpdate({
      progress: 75,
      status: "reserving",
      message: "Reserving inventory"
    });
    
    // Reserve inventory
    const reservationId = await inventoryDb.createReservation(orderId, productId, quantity);
    
    // Final progress update
    await task.pushUpdate({
      progress: 100,
      status: "completed",
      message: "Inventory reserved successfully"
    });
    
    return {
      success: true,
      reservationId: reservationId
    };
  } catch (error) {
    throw error;
  }
});

// Compensation handler with context awareness
inventoryService.onRevert(async (task, result, context) => {
  console.log(`Reverting reservation for order ${task.input.orderId}`);
  
  // Use context to determine compensation behavior
  if (context) {
    console.log(`Compensation triggered by: ${context.reason}`);
    console.log(`Orchestration ID: ${context.orchestrationId}`);
    
    // Log additional information for aborted tasks
    if (context.reason === 'aborted' && context.payload) {
      console.log(`Abort reason: ${context.payload.reason}`);
      
      // For inventory-specific aborts, we might not need to do anything
      if (
        context.payload.reason === 'INSUFFICIENT_INVENTORY' && 
        context.payload.productId === task.input.productId
      ) {
        console.log('No actual reservation was made due to insufficient inventory');
        return { status: 'completed' };
      }
    }
  }
  
  // For other cases where a reservation was made, release it
  if (result && result.reservationId) {
    try {
      await inventoryDb.releaseReservation(result.reservationId);
      console.log(`Released reservation ${result.reservationId}`);
      return { status: 'completed' };
    } catch (error) {
      console.error(`Failed to release reservation: ${error.message}`);
      return { 
        status: 'failed',
        error: error.message 
      };
    }
  } else {
    // No reservation ID found
    console.log('No reservation to revert');
    return { status: 'completed' };
  }
});

// Handle graceful shutdown
process.on('SIGINT', async () => {
  console.log('Shutting down inventory service...');
  inventoryService.shutdown();
  process.exit(0);
});
```

## Next Steps

1. Review the example projects for more integration patterns
2. Join our community for support and updates
3. Check out the action orchestration guide to start using your services

Need help? Contact the orra team or open an issue on GitHub.
