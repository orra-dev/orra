# 🪡Orra

Orra is a language-agnostic LLM powered orchestration platform. It provides building blocks to build reliable and fast
multi-agent applications. Developers can stop re-inventing the wheel and focus on adding customer value.

## Current Release: Narwal 🐋🦄

Our current release is codenamed "Narwal". This release brings the ✨Alpha✨ version of Orra's orchestration capabilities.

![Our Narwhal](releases/images/0.1.0/narwhal.jpeg)

## Install

### Prerequisites

- Docker and Docker Compose - For running the control plane
- Node.js 18+ and npm - For running example services
- An OpenAI API key - For LLM-powered orchestration

### 1. Install Orra CLI

Download the latest CLI binary for your platform from our releases page.

```shell
# macOS
curl -L https://github.com/ezodude/orra/releases/download/v0.1.0-narwhal/orra-macos -o /usr/local/bin/orra
chmod +x /usr/local/bin/orra

# Linux
curl -L https://github.com/ezodude/orra/releases/download/v0.1.0-narwhal/orra-linux -o /usr/local/bin/orra
chmod +x /usr/local/bin/orra

# Verify installation
orra version
```

### 2. Get Orra Running

Clone the repository and start the control plane:

```shell
git clone https://github.com/ezodude/orra.git
cd orra/controlplane

# Set your OpenAI API key
echo "OPENAI_API_KEY=your-key-here" > .env

# Start the control plane
docker-compose up -d
```

## Quick Start

In this guide, you'll set up a dynamically orchestrated app using Orra! The app echos values using an Echo JS based
service.

This is a simple but powerful setup that demonstrates Orra's key capabilities:

- Dynamic orchestration
- Resilient execution

### 1. Configure Your Workspace

```shell
# Create a new project
orra projects add my-orra-project

# Register a webhook
orra webhooks add http://localhost:8080/webhook

# Create an API key for your services
orra api-keys gen service-key
```

Open a new terminal window or tab, and run a webhook server

```shell
# Start the webhook as a server using the verify subcommand (verify includes helpers to verify your Orra setup)
orra verify webhooks start http://localhost:8080/webhook
```

### 2. Start the Example Echo Service

Open a new terminal window or tab to run the Echo service.

```shell
ORRA_API_KEY='<value of API key created earlier>'
ORRA_URL=http://localhost:8005

cd examples/echo-js
npm install
npm run dev
```

### Demonstrating Orra

Our Echo service might seem simple, but it's perfect for demonstrating Orra's magic. Let's create a fun sequence:

1. Send your first message:

```shell
orra verify tell 'Echo this secret message' --data message:'🎯 Target acquired!'
```

Watch the magic happen:

- Orra analyzes your action using AI
- Creates an execution plan
- Orchestrates the service
- Delivers results to your webhook

2. Let's break things (intentionally):

```shell
# STOP THE ECHO SERVICE (Ctrl+C in its terminal)

# Send another message
orra verify tell 'Echo the rescue signal' --data message:'🆘 Send help!'

# Check what's happening
orra inspect -d <orchestration-id>
```

You'll see Orra patiently waiting, monitoring the service's health.

3. Restore the service and watch recovery:

```shell
# Restart the Echo service (in its terminal)
npm run dev

# Check the orchestration again
orra inspect -d <orchestration-id>
```

Marvel as your message completes its journey! This demonstrates Orra's built-in resilience - no special error handling
code needed.

### What Just Happened?

You've just experienced:

- Dynamic orchestration using AI - no messy task routing logic insight
- Automatic service health monitoring
- Built-in resilient execution
- Real-time status tracking
- Webhook result delivery

The best part? This same pattern works for complex multi-service and multi-agent scenarios. Orra handles the complexity
while you focus on building your application.

## Key Benefits

1. **Dynamic AI-Powered Adaptability**: Automatically adjust workflows in real-time based on context, ensuring optimal
   performance in changing environments.

2. **Build Reliable Multi-Agent Systems**: Create robust applications with deterministic task completion and automatic
   outage handling. **\[Coming Soon\]**

3. **Accelerate Task Execution**: Enable fast parallel processing for concurrent execution of services and Agents.

4. **Integrate Across Languages**: Utilize our SDKs to reliably orchestrate any service or Agent, regardless of
   programming language. **Starting with JavaScript/TypeScript**

5. **Manage Results Comprehensively**: Track interim results across services/Agents and accurately combine them into a
   single final output.

## Why LLM-Powered Orchestration?

LLM-powered orchestration enables real-time adjustment of workflows based on context, intermediate results, and changing
requirements, allowing for more intelligent and flexible multi-agent systems. Here's how it enhances AI workflow
management:

1. **Intelligent Workflow Generation**: Automatically create and adapt orchestration plans by understanding the
   capabilities of registered services and agents.
    - Decomposes complex tasks without manual intervention
    - Allocates resources optimally based on service capabilities
    - Handles task dependencies efficiently

2. **Dynamic Execution and Adaptation**: Continuously adjust workflows during runtime, responding to evolving conditions
   and interim outputs.
    - Optimizes task sequences in real-time
    - Implements intelligent error handling and recovery
    - Reallocates resources flexibly based on performance and changing needs

## Installation

**Orra is in Alpha**. The core component is the control plane which is run as a server. It is available for Self-hosting
in Single User mode. We do not recommend running it in production yet.

You need to
have [Git](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git), [Docker](https://docs.docker.com/desktop/)
and [Docker Compose](https://docs.docker.com/compose/install/) installed before you start.

1. Clone the repo.
    ```shell
      git clone https://github.com/ezodude/orra
    ```
2. Navigate to the control plane's root folder and run the control plane.
    ```shell
      cd control-plane
      cp env-example .env 
      docker compose up -d
    ```
3. Download the relevant Orra CLI binary and add it your path.
    ```shell
      mv orra /user/local/bin/.
    ```
4. Login with the CLI and follow the instructions
    ```shell
      orra login
    ```

## Using the Orra CLI

Run commands to set up projects, inspect orchestrations and generate API keys using the Orra command-line tool.

```shell
orra --help
# orra manages Orra and orchestration workflows. 
# Usage:  orra [OPTIONS] COMMAND
# projects    Add and manage projects
# webhooks    Add and manage webhooks for a project
# api-keys    Add and manage API keys for a project
# ps          List orchestrations for a project
# inspect     Return information of an orchestration
# logs        Fetch the logs for an orchestration
# login       Log in to a registry
# logout      Log out from a registry
# version     Print the client and server version information
```

## Getting started with orchestration

A multi-agent application may consist of many components. Orra orchestrates the LLM based Agents and related services,
e.g. data ingestion services, that run the core of your application. It assumes both Agents and services are run as
microservices, always available to accept data.

Orchestration plans are constantly adjusted to ensure tasks are completed using the available Agents and services. For
example, if a necessary service is offline, task execution maybe halted until it's back online.

It is up to you how your Agents and services are deployed. But, we strongly recommend they are deployed and run as
containers for optimal performance.

### Configure orchestration for your project

1. Log in and add a project.
   ```shell
      orra projects add new-orra-project
   ```
2. Add a webhook to accept orchestration results.
   ```shell
      orra webhooks add --url "http://localhost:3000/webhooks/orra" -p new-orra-project
   ```
3. Generate an API key to authenticate and orchestrate tasks. The new API key is required for use in Orra SDKs.
   ```shell
      orra api-keys add --name 'My API Key' -p new-orra-project
   ```

### Setup Agents and services for orchestration

You can use a preferred language SDK to register your Agents and services with our control plane.

Here's an example using the JavaScript SDK.

1. Give your Agent or service a unique name.
   Ensure the name can be used as a DNS subdomain name as defined in RFC 1123. This means the name must:
    - contain no more than 253 characters
    - contain only lowercase alphanumeric characters, '-' or '.'
    - start with an alphanumeric character
    - end with an alphanumeric character

2. Give your Agent or service a concise description that clearly explain what it does.
   The description cannot be longer than 500 chars.

3. Define the expected input and output schema.
   ```javascript
   const serviceSchema = {
     input: {
       type: 'object',
       fields: [ { name: 'customerId', type: 'string', format: 'uuid' } ],
       required: [ 'customerId' ]
     },
     output: { 
       type: 'object',
       fields: [ 
         { name: 'id', type: 'string', format: 'uuid' },
         { name: 'name', type: 'string' },
         { name: 'balance', type: 'number', minimum: 0 }
       ]
     }
   };
   ```

4. Set up the task handler, this is a function called by the SDK that will kick off your Agent or service's work.
    - It will receive an input object that conforms to the agent/service input schema.
    - It will output data as an object that conforms to the agent/service output schema.

5. Add a version to the service, this useful for logging and general system debugging.

<details>
<summary>Here's the full service setup for Orra orchestration.</summary>

```javascript
const { createClient } = require('@orra/sdk');

// Create a client
const orraClient = createClient({
	orraUrl: process.env.ORRA_URL,
	orraKey: process.env.ORRA_API_KEY
});

// Flesh out your services inputs and outputs
const serviceSchema = {
	input: {
		type: 'object',
		fields: [ { name: 'customerId', type: 'string', format: 'uuid' } ],
		required: [ 'customerId' ]
	},
	output: {
		type: 'object',
		fields: [
			{ name: 'id', type: 'string', format: 'uuid' },
			{ name: 'name', type: 'string' },
			{ name: 'balance', type: 'number', minimum: 0 }
		]
	}
};

async function main() {
	try {
		// Register your service or Agent, clearly explain what it does.
		await orraClient.registerService(
			'CustomerAccountService',
			{
				description: 'Retrieves and manages customer account data',
				schema: serviceSchema,
				version: '1.0.0'
			}
		);
		console.log('Service registered successfully');
		orraClient.startHandler(handler)
	} catch (error) {
		console.error('Registration failed:', error);
	}
}

// This will receive input as per the input schema setup previously
function handler(taskData) {
	console.log('Received task:', task);
	
	// Process the task
	// ..
	
	return { status: 'completed', result: 'Processed data' };
}

main();

process.on('SIGINT', () => {
	console.log('Closing connection...');
	orraClient.close();
	process.exit();
});
```

</details>

### Orchestrate tasks for your app

Typically, you would orchestrate tasks from a front-end client, a CRON Job or another backend system. The tasks may
encompass short queries to day-long running tasks.

Simply issue a `POST` request to Orra detailing a job that needs to be done by your application and include any related
data, like a customerId, in the payload. Make sure you include your Orra `API key` as ties the job to a specific
project.

This is an async request that will result in a response with an Accepted (`202`) HTTP Status Code. The response will
also include,

- A payload that details Orra's initial orchestration plan.
- A webhook url that will receive the final job result - already added through the CLI.

<details>
   <summary>This is an example request.</summary>

```shell
curl -X POST https://api.orra.dev/orchestrations \
-H "Content-Type: application/json" \
-H "Authorization: Bearer YOUR_API_KEY_HERE" \
-d '{
  "action": {
    "type": "customer-inquiry",
    "content": "Hi, I am interested in your product. Can you tell me more about its features?"
  },
  "data": [
    {
      "field": "customerId",
      "value": "cust12345",
    },
    {
      "field": "sellerId",
      "value": "sell98765",
    }
  ],
  "timestamp": "2024-09-06T14:30:00Z"
}'
```

</details>

<details>
<summary>This generates an orchestration plan as part of the response.</summary>

> [!NOTE]  
> Some components will be executed in **parallel** as denoted by the **parallelExecutionGroups** attribute and the
> dependencies attribute for each step.

```shell
HTTP/1.1 202 Accepted
Content-Type: application/json

{
  "status": "ACCEPTED",
  "executionId": "exec-2024-09-06-15:45:30-789",
  "message": "Request accepted. Orchestration plan created.",
  "resultTarget": {
    "webhook": "https://my-app-frontend/webhooks/orra"
  },
  "plan": {
    "steps": [
      {
        "id": "step1",
        "component": "CustomerDataService",
        "action": "retrieveCustomerProfile",
        "input": {
          "customerId": "cust12345"
        },
        "expectedOutput": "customerProfile",
        "dependencies": []
      },
      {
        "id": "step2",
        "component": "ProductCatalogService",
        "action": "getRelevantProducts",
        "input": {
          "inquiry": "{{request.action.content}}"
        },
        "expectedOutput": "relevantProducts",
        "dependencies": []
      },
      {
        "id": "step3",
        "component": "SellerDataService",
        "action": "retrieveSellerProfile",
        "input": {
          "sellerId": "sell98765"
        },
        "expectedOutput": "sellerProfile",
        "dependencies": []
      },
      {
        "id": "step4",
        "component": "AIAssistantService",
        "action": "generateResponse",
        "input": {
          "customerProfile": "{{step1.output.customerProfile}}",
          "relevantProducts": "{{step2.output.relevantProducts}}",
          "sellerProfile": "{{step3.output.sellerProfile}}",
          "inquiry": "{{request.action.content}}"
        },
        "expectedOutput": "aiGeneratedResponse",
        "dependencies": ["step1", "step2", "step3"]
      }
    ],
    "parallelExecutionGroups": [
      ["step1", "step2", "step3"],
      ["step4"],
    ],
    "estimatedCompletionTime": "2024-09-06T15:45:45Z",
  }
}
```

</details>

<details>
<summary>Finally, a client receives the result of the orchestration using the <b>webhook</b>.</summary>

```shell
HTTP/1.1 200 OK
Content-Type: application/json

{
  "executionId": "exec-2024-09-06-15:45:30-789",
  "status": "COMPLETED",
  "startTime": "2024-09-06T15:45:30Z",
  "endTime": "2024-09-06T15:45:43Z",
  "result": {
	  "content": "Thank you for your interest in our products! Based on your inquiry, I'd recommend our Premium Analytics Suite, which offers advanced features including real-time data processing, customizable dashboards, and AI-driven insights. It seamlessly integrates with popular CRM systems and provides comprehensive reporting tools. Would you like to schedule a demo to see these features in action?",
	  "sentTimestamp": "2024-09-06T15:45:42Z"
  },
  "stepResults": [
    {
      "id": "step1",
      "component": "CustomerDataService",
      "status": "COMPLETED",
      "startTime": "2024-09-06T15:45:31Z",
      "endTime": "2024-09-06T15:45:32Z",
      "output": {
        "customerProfile": {
          "id": "cust12345",
          "name": "Jane Doe",
          "company": "TechCorp Inc.",
          "industry": "Software"
        }
      }
    },
    {
      "id": "step2",
      "component": "ProductCatalogService",
      "status": "COMPLETED",
      "startTime": "2024-09-06T15:45:31Z",
      "endTime": "2024-09-06T15:45:33Z",
      "output": {
        "relevantProducts": [
          {
            "id": "prod001",
            "name": "Premium Analytics Suite",
            "features": ["Real-time processing", "AI-driven insights", "CRM integration"]
          }
        ]
      }
    },
    {
      "id": "step3",
      "component": "SellerDataService",
      "status": "COMPLETED",
      "startTime": "2024-09-06T15:45:31Z",
      "endTime": "2024-09-06T15:45:32Z",
      "output": {
        "sellerProfile": {
          "id": "sell98765",
          "name": "John Smith",
          "department": "Enterprise Sales"
        }
      }
    },
    {
      "id": "step4",
      "component": "AIAssistantService",
      "status": "COMPLETED",
      "startTime": "2024-09-06T15:45:33Z",
      "endTime": "2024-09-06T15:45:38Z",
      "output": {
        "aiGeneratedResponse": "Thank you for your interest in our products! Based on your inquiry, I'd recommend our Premium Analytics Suite, which offers advanced features including real-time data processing, customizable dashboards, and AI-driven insights. It seamlessly integrates with popular CRM systems and provides comprehensive reporting tools. Would you like to schedule a demo to see these features in action?"
      }
    },
  ]
}
```

</details>
