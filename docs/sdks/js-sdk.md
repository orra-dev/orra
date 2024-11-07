# Orra JS SDK Documentation

Orra makes it easy to add resilient, production-ready orchestration to your existing AI services and agents. This guide will help you integrate the Orra SDK with your Node.js applications.

## Installation

```bash
# Clone the Orra repository
git clone https://github.com/your-org/orra.git

# Install the SDK from local repository
npm install -S ../path/to/repo/orra/sdks/js
```

## Quick Integration Example

The Orra SDK is designed to wrap your existing service logic with minimal changes. Here's a simple example showing how to integrate an existing chat service:

```javascript
import { createClient } from '@orra/sdk';
import { myService } from './existing-service';  // Your existing logic

// Initialize the Orra client
const client = createClient({
  orraUrl: process.env.ORRA_URL,
  orraKey: process.env.ORRA_API_KEY
});

// Register your service
await client.registerService('Customer Chat Service', {
  description: 'Handles customer chat interactions',
  schema: {
    input: {
      type: 'object',
      properties: {
        customerId: { type: 'string' },
        message: { type: 'string' }
      },
      required: ['customerId', 'message']
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
client.startHandler(async (task) => {
  try {
    const { customerId, message } = task.input;
    
    // Use your existing chat service function
    // Your function handles its own retries and error recovery
    const response = await myService(customerId, message);
    
    return { response };
  } catch (error) {
    // Once you determine the task should fail, throw the error
    // Orra will handle failure propagation to the control plane
    throw error;
  }
});
```

## Understanding the SDK

The Orra SDK follows patterns similar to serverless functions or job processors, making it familiar for AI Engineers. Your services become event-driven handlers that:

1. Register capabilities with Orra (what they can do)
2. Process tasks when called (actual execution)
3. Return results for orchestration

### Key Concepts

- **Services vs Agents**: Both use the same SDK but are registered differently
    - Services: Stateless, function-like handlers (e.g., chat services, data processors)
    - Agents: Stateful, long-running processes (e.g., AI assistants, monitoring agents)

- **Schema Definition**: Similar to OpenAPI/GraphQL schemas, defines inputs/outputs
- **Handler Functions**: Like serverless functions, process single tasks
- **Health Monitoring**: Automatic health checking and reporting
- **Service Identity**: Maintained across restarts and updates

## Detailed Integration Guide

### 1. Service Registration

Register your service with its capabilities:

```javascript
const client = createClient({
  orraUrl: process.env.ORRA_URL,
  orraKey: process.env.ORRA_API_KEY
});

// For stateless services
await client.registerService('Service Name', {
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
await client.registerAgent('Agent Name', {
  description: 'What this agent does',
  schema: {
    // Same schema structure as services
  }
});
```

### 2. Task Handler Implementation

Implement your task handler to process requests:

```javascript
client.startHandler(async (task) => {
  try {
    // 1. Access task information
    const { input, executionId } = task;
    
    // 2. Your existing business logic with its own retry/recovery
    const result = await yourExistingFunction(input);
    
    // 3. Return results
    return result;
  } catch (error) {
    // After your error handling is complete, let Orra know about permanent failures
    throw error;
  }
});
```

### 3. Error Handling

Handle errors in your business logic:

```javascript
client.startHandler(async (task) => {
  try {
    // Your retry logic here
    const result = await withRetries(() => riskyOperation());
    return result;
  } catch (error) {
    // After your retries are exhausted, throw the error
    // Orra will handle the failure in the orchestration
    throw new Error(`Operation failed: ${error.message}`);
  }
});

// Example retry implementation
async function withRetries(operation, maxAttempts = 3) {
  let lastError;
  
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    try {
      return await operation();
    } catch (error) {
      lastError = error;
      if (attempt < maxAttempts) {
        await sleep(Math.pow(2, attempt) * 1000); // Exponential backoff
      }
    }
  }
  
  throw lastError;
}
```

## Advanced Features

### Persistence Configuration

Orra maintains service identity across restarts using persistence. This is crucial for:
- Maintaining service history
- Ensuring consistent service identification
- Supporting service upgrades

```javascript
const client = createClient({
  orraUrl: process.env.ORRA_URL,
  orraKey: process.env.ORRA_API_KEY,
  persistenceOpts: {
    // 1. File-based persistence (default)
    method: 'file',
    filePath: './custom/path/service-key.json',
    
    // 2. Custom persistence (e.g., database)
    method: 'custom',
    customSave: async (serviceId) => {
      await db.services.save(serviceId);
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
    - Only propagate errors to Orra after recovery attempts are exhausted

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
import { analyzeImage } from './ai-service';

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
import { createClient } from '@orra/sdk';
import { analyzeImage } from './ai-service';  // Reuse existing logic

const client = createClient({
  orraUrl: process.env.ORRA_URL,
  orraKey: process.env.ORRA_API_KEY
});

await client.registerService('Image Analysis Service', {
  description: 'Analyzes images using AI',
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
client.startHandler(async (task) => {
  try {
    const { imageUrl } = task.input;
    // Your function handles its own retries
    return await analyzeImage(imageUrl);
  } catch (error) {
    // After your error handling is complete, let Orra know about the failure
    throw error;
  }
});
```

## Next Steps

1. Review the example projects for more integration patterns
2. Join our community for support and updates
3. Check out the action orchestration guide to start using your services

Need help? Contact the Orra team or open an issue on GitHub.
