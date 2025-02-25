# Echo Service Example (JavaScript)

A minimal example demonstrating how to build and orchestrate a service using Orra. Perfect for learning the basics of service orchestration.

```mermaid
sequenceDiagram
    participant CLI as Orra CLI
    participant CP as Control Plane
    participant ES as Echo Service
    participant WH as Webhook Server

    CLI->>CP: Send action
    CP->>ES: Orchestrate task
    ES->>CP: Return echo
    CP->>WH: Send result
    Note over WH: See result in terminal
```

## ✨ Features

- 🔄 Basic service registration and orchestration
- 📡 Real-time WebSocket communication
- ⚡ Reliable message delivery
- 🛡️ Built-in health monitoring
- 🚀 Simple but production-ready patterns

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and [Docker Compose](https://docs.docker.com/compose/install/)
- [OpenAI API key](https://platform.openai.com/api-keys) for Orra's control plane

## Setup

1. First, setup Orra by following the [Installation instructions](../../README.md#installation):
```bash
# Clone Orra
git clone https://github.com/ezodude/orra
cd orra/controlplane

# Set your OpenAI API key
echo "OPENAI_API_KEY=your-key-here" > .env

# Start the control plane
docker compose up
```

2. Setup your Orra project:
```bash
# Install Orra CLI 
curl -L https://github.com/ezodude/orra/releases/download/v0.2.1/orra-darwin-arm64 -o /usr/local/bin/orra
chmod +x /usr/local/bin/orra

# Create project, add a webhook and API key
orra projects add my-echo-app
orra webhooks add http://host.docker.internal:8888/webhook
orra api-keys gen echo-key
```

3. Configure the Echo service:
```bash
cd examples/echo
echo "ORRA_API_KEY=key-from-step-2" > .env
```

## Running the Example

1. Start the webhook server (in a separate terminal):
```bash
# Start the webhook server using the verify subcommand
orra verify webhooks start http://localhost:8888/webhook
```

2. Start the Echo service:
```bash
# Start Echo Service
docker compose up
```

3. Try it out:
```bash
# Send a test message
orra verify run "Echo this message" --data message:"Hello from Orra!"

# Check the result
orra ps
orra inspect <orchestration-id>
```

You should see the result both in the webhook server terminal and through the inspect command.

## SDK Integration Example
Here's the complete Echo service implementation showing how simple Orra integration can be:

```javascript
import { createClient } from '@orra.dev/sdk';
import schema from './schema.json' assert { type: 'json' };

const orra = createClient({
	orraUrl: process.env.ORRA_URL,
	orraKey: process.env.ORRA_API_KEY,
	persistenceOpts: getPersistenceConfig()
});

// Health check
app.get('/health', (req, res) => {
	res.status(200).json({ status: 'healthy' });
});

async function startService() {
	try {
		// Register the echo service with Orra
		await orra.registerService('echo-service', {
			description: 'A simple service that echoes back the first input value it receives.',
			schema
		});
		
		orra.startHandler(async (task) => {
			console.log('Echoing input:', task.id);
			const message = task?.input
			return { echo: message };
		});
		
		console.log('Echo Service started successfully');
	} catch (error) {
		console.error('Failed to start Echo Service:', error);
		process.exit(1);
	}
}

// Start the Express server and the service
app.listen(port, () => {
	console.log(`Server listening on port ${port}`);
	startService().catch(console.error);
});
```

That's it! Orra provides:
- Service discovery
- Health monitoring
- Reliable task execution
- Error recovery

## Learn More

- [Orra Documentation](../../docs)
- [CLI Documentation](../../docs/cli.md)
