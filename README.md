# 🪡Orra

Build production-ready multi-agent applications without the complexity. Orra uses LLMs to dynamically orchestrate your
services and agents, handling reliability and performance so you can focus on building features that matter.

## Current Release: Narwal 🐋🦄 (Alpha)

Our current release is codenamed "Narwal". This release brings the ✨Alpha✨ version of Orra's orchestration capabilities.

## Why Orra

- **Dynamic LLM Orchestration**: Stop hard-coding agent workflows. Orra automatically creates and adapts execution plans
  based on your agents' plus services' capabilities and real-time context.

- **Production-Ready Reliability**: Built-in fault tolerance with automatic retries, health checks, and stateful
  execution tracking. No more building your own reliability layer.

- **High-Throughput Performance**: Parallel execution, efficient task routing, and smart caching ensure your multi-agent
  apps stay responsive under load.

## Install

### Prerequisites

- [Docker](https://docs.docker.com/desktop/) and [Docker Compose](https://docs.docker.com/compose/install/) - For
  running the control plane
- [Node.js 18+](https://nodejs.org/en/download/package-manager) - For running example services
- An [OpenAI API key](https://platform.openai.com/docs/quickstart) - For LLM-powered orchestration

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

Build your first AI-orchestrated application! We'll use our [Echo service](examples/echo-js) example to show you the
magic of intelligent service orchestration.

While simple, it showcases Orra's capabilities:

- **Dynamic orchestration**: AI analyzes your instructions and creates execution plans - no manual routing needed.
- **Resilient execution**: Service interruptions, retries, and recovery handled automatically - zero special handling
  code.

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

Open a new terminal window or tab to run the Echo service (orchestrated using the [JS SDK](sdks/js)).

```shell
ORRA_API_KEY='<value of API key created earlier>'
ORRA_URL=http://localhost:8005

cd examples/echo-js
npm install
npm run dev
```

### 3. Demonstrating Orra

Let's create a fun sequence:

__Send your first message__

```shell
orra verify tell 'Echo this secret message' --data message:'🎯 Target acquired!'
```

Watch the magic happen:

- Orra analyzes your action using AI
- Creates an execution plan
- Orchestrates the service
- Delivers results to your webhook

__Let's break things (intentionally)__

```shell
# STOP THE ECHO SERVICE (Ctrl+C in its terminal)

# Send another message
orra verify tell 'Echo the rescue signal' --data message:'🆘 Send help!'

# Check what's happening
orra inspect -d <orchestration-id>
```

You'll see Orra patiently waiting, monitoring the service's health.

__Restore the service and watch recovery__

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

- 🤖 Dynamic orchestration using AI
- ⛑️ Automatic service health monitoring ️
- 🦾 Built-in resilient execution
- 🔮 Real-time status tracking
- 🪝 Webhook result delivery

The best part? This same pattern works for complex multi-service and multi-agent scenarios. Orra handles the complexity
while you focus on building your application.

## Alpha Features & Limitations

### Available Now

* LLM-powered task decomposition and routing
* In-memory execution tracking with exactly-once guarantees
* Smart service health handling with execution pausing and heartbeat monitoring
* Short-term retries with exponential backoff (up to 5 attempts)
* Simple JavaScript SDK with TypeScript support
* CLI for Orra-powered projects management
* Automatic parallel execution optimization
* Built-in service discovery

### Current Limitations

1. **Storage**: All state is in-memory and will be lost on control plane restart
2. **Deployment**: Single-instance only, designed for local development
3. **Recovery**: Limited to individual service recovery
4. **SDKs**: JavaScript/TypeScript only

### Coming Soon

* Persistent storage
* Additional language SDKs
* Streaming for task processing
* Continuous adjustment of Agent workflows during runtime
* Resource Reallocation based on performance and changing needs
* Distributed deployment
  .. and many more planned

## Examples

- 📱 [Chat Application](examples/ecommerce-chat-js) - E-commerce customer service with a delivery specialised agent
- 🔄 [Echo Service](examples/echo-js) - Simple example showing core concepts

## Join Our Alpha Testing Community

**We're looking for developers who:**

- Are building multi-agent applications
- Want to help shape Orra's development
- Are comfortable working with Alpha software
- Can provide feedback on real-world use cases

**Connect With Us:**

- GitHub Discussions - Share your experience and ideas
- Office Hours - Weekly calls with the team

## License

Orra is MPL-2.0 licensed.
