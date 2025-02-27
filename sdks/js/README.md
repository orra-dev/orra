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
await echoToolSvc.register({
  description: 'A service with compensation support',
  revertible: true,
  schema: {
    // Schema definition
  }
});

// Add compensation handler
echoToolSvc.onRevert(async (originalTask, taskResult) => {
  console.log(`Reverting task ${originalTask.id}`);
  // Compensate the action
});
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
