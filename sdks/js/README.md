# Orra SDK for JavaScript

JavaScript SDK for [Orra](https://github.com/orra-dev/orra) - Build reliable multi-agent applications that handle complex real-world interactions.

## Installation

```bash
npm install -S @orra.dev/sdk
```

## Usage

```javascript
import { initService } from '@orra.dev/sdk';

// Initialize the SDK
const echoToolSvc = initService({
	name: 'echo',
	orraUrl: process.env.ORRA_URL,
	orraKey: process.env.ORRA_API_KEY
});

// Register your service
await echoToolSvc.register({
	description: 'A simple echo provider that echoes whatever its sent',
	schema: {
		input: {
			type: 'object',
			properties: {
				message: { type: 'string' }
			},
			required: [ 'message' ]
		},
		output: {
			type: 'object',
			properties: {
				response: { type: 'string' }
			}
		}
	}
});

// Define your service handler
echoToolSvc.start(async (task) => {
	try {
		const { message } = task.input;
		return { response: `Echo: ${message}` };
	} catch (error) {
		throw error;
	}
});

// Handle graceful shutdown
process.on('SIGINT', async () => {
	console.log('Shutting down...');
	echoToolSvc.shutdown();
	process.exit(0);
});
```

## Advanced Features

### Revertible Services with Compensations

```javascript
// Register as revertible
await inventoryService.register({
	description: 'An inventory service with compensation support',
	revertible: true,
	schema: {
		// Schema definition
	}
});

// Compensation handler with context
inventoryService.onRevert(async (originalTask, taskResult, context) => {
	console.log(`Reverting task ${originalTask.id}`);
	
	// Access compensation context
	if (context) {
		console.log(`Compensation reason: ${context.reason}`);
		console.log(`Orchestration ID: ${context.orchestrationId}`);
		
		// For aborted tasks, access the abort payload
		if (context.reason === 'ABORTED' && context.payload) {
			console.log(`Abort reason: ${context.payload.reason}`);
		}
	}
	
	// Perform compensation logic
	await releaseReservation(taskResult.reservationId);
});
```

The compensation handler receives a third parameter context that provides information about why the compensation was triggered:
- `reason`: Why compensation was triggered (e.g., 'aborted', 'failed')
- `orchestrationId`: The ID of the parent orchestration
- `payload`: Additional data related to the compensation (optional)
- `timestamp`: When the compensation was initiated

### Aborting Tasks

The SDK allows you to abort task processing when needed, instead of completing the task normally:

```javascript
// Handler with abort capability
service.start(async (task) => {
	try {
		// Check condition that might require abort
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

When a task is aborted, the orchestration will receive an abort message with your payload, and task processing will stop immediately. In revertible services, the abort information will be available during compensation.

### Progress Updates for Long-Running Tasks

```javascript
// Handler with progress updates
service.start(async (task) => {
  try {
    // Begin processing
    await task.pushUpdate({
      progress: 25,
      status: "processing",
      message: "Starting data processing"
    });
    
    // Run time-consuming operation
    await processFirstBatch();
    
    // Report halfway progress
    await task.pushUpdate({
      progress: 50,
      status: "processing",
      message: "Halfway complete"
    });
    
    // Complete the processing
    await processSecondBatch();
    
    // Final progress update
    await task.pushUpdate({
      progress: 100,
      message: "Processing complete"
    });
    
    return { success: true, data: "Task completed successfully" };
  } catch (error) {
    throw error;
  }
});
```

Progess updates allow you to send interim results to provide visibility into long-running tasks. View these updates using the CLI:

```bash
# View summarized updates (first/last)
orra inspect -d <orchestration-id> --updates

# View all detailed updates
orra inspect -d <orchestration-id> --long-updates
```

### Custom Persistence

```javascript
const toolSvc = initService({
	name: 'my-tools-as-service',
	orraUrl: process.env.ORRA_URL,
	orraKey: process.env.ORRA_API_KEY,
	persistenceOpts: {
		// Custom file path
		method: 'file',
		filePath: './custom/path/service-key.json',
		
		// Or custom persistence implementation
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

## Documentation

For more detailed documentation, please visit [Orra JS SDK Documentation](https://github.com/orra-dev/orra/blob/main/docs/sdks/js-sdk.md).
