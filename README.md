# 🪡Orra (✨Alpha✨)

Move beyond simple Crews and Agents. Build production-ready multi-agent applications that handle complex real-world
interactions. Orra orchestrates your services and agents with built-in reliability and intelligent reasoning — across
any language, agent framework or deployment platform.

Prototype rapidly today, scale to production tomorrow. No complex infrastructure required.

## Releases

### Manta Ray 🐠🌊️ - January 2025 🎉

Brings in a robust Compensation Guardrails system with JS and Python SDK updates to match.

**Other Features**

- Service/Agent stability improvements.
- Major fixes across control plane and SDKs.

[[View all releases](https://github.com/orra-dev/orra/releases) →]

## Why Orra

- **Workflow intelligence**: Orra automatically coordinates both your AI and general services by understanding their
  capabilities and adapting execution in real-time - you can focus on building features instead of managing complex
  interactions.

- **Production-Ready Reliability**: Built-in fault tolerance and durability with automatic retries, health checks,
  compensations and stateful execution tracking. No more building your own reliability layer.

- **High-Throughput Performance**: Parallel execution, efficient task routing, and smart caching ensure your multi-agent
  apps stay responsive under load.

## The Multi-Agent Development Reality

If you're building multi-agent applications, this probably sounds familiar:

**Production Reliability**: Your agent app works perfectly in demos, but in production it's brittle. One hiccup in a
chain of agent calls and everything falls apart.

**Workflow Hell**: Your code is a maze of hard-wired sequences between agents and services. Adding a new integration or
changing a workflow means rewriting orchestration logic, updating schemas, and praying you didn't break existing flows.

**Scaling Pains**: Scaling beyond a few concurrent users means juggling queues, caches, and distributed state across
agents and backend services. What started as "just a few agents talking to each other" has become a distributed systems
problem.

**LLM Chain Explosion**: Your research agents are drowning in unnecessary LLM function calls. Each extra call adds
latency, risks hallucination, and degrades reliability. What should be precise AI interactions become endless chains
of "just one more try."

Orra gives you the power to handle complex real-world interactions with your agents and services. No rewrites, no
infrastructure headaches - just predictable, intelligent routing that gets work done.

## Install

### Prerequisites

- [Docker](https://docs.docker.com/desktop/) and [Docker Compose](https://docs.docker.com/compose/install/) - For
  running the control plane
- An [OpenAI API key](https://platform.openai.com/docs/quickstart) - For LLM-powered orchestration

> **Note**: The control plane currently runs in-memory. If restarting, follow
> our [Reset Guide](docs/reset-control-plane.md).

### 1. Install Orra CLI

Download the latest CLI binary for your platform from our releases page.

```shell
# macOS
curl -L https://github.com/ezodude/orra/releases/download/v0.1.3-narwhal/orra-macos -o /usr/local/bin/orra
chmod +x /usr/local/bin/orra

# Linux
curl -L https://github.com/ezodude/orra/releases/download/v0.1.3-narwhal/orra-linux -o /usr/local/bin/orra
chmod +x /usr/local/bin/orra

# Verify installation
orra version
```

→ [Full CLI documentation](docs/cli.md)

### 2. Get Orra Running

Clone the repository and start the control plane:

```shell
git clone https://github.com/ezodude/orra.git
cd orra/controlplane

# Set your OpenAI API key
echo "OPENAI_API_KEY=your-key-here" > .env

# Start the control plane
docker compose up --build
```

## Quick Start

Build your first reliable AI application with Orra! We'll use our [Echo service (JavaScript)](examples/echo-js) example
to show you the magic of intelligent service orchestration.
Requires [Node.js 18+](https://nodejs.org/en/download/package-manager).

While simple, it showcases Orra's capabilities:

- **Workflow intelligence**: AI analyzes your instructions and creates execution plans - no manual routing needed.
- **Durable execution**: Service interruptions, retries, and recovery handled automatically - zero special handling
  code.

**If Python is more your speed**, follow along using the [Echo service (Python)](examples/echo-python) example.

### 1. Configure Your Workspace

```shell
# Create a new project
orra projects add my-ai-app

# Register a webhook
orra webhooks add http://host.docker.internal:8080/webhook

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

__Run your first action__

```shell
orra verify run 'Echo this secret message' --data message:'🎯 Target acquired!'
```

Watch the magic happen:

- Orra analyzes your action using AI
- Creates an execution plan
- Orchestrates the service
- Delivers results to your webhook

__Let's break things (intentionally)__

```shell
# STOP THE ECHO SERVICE (Ctrl+C in its terminal)

# Run another action
orra verify run 'Echo the rescue signal' --data message:'🆘 Send help!'

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
- 🦾 Built-in durable execution
- 🔮 Real-time status tracking
- 🪝 Webhook result delivery

The best part? This same pattern works for complex multi-service and multi-agent scenarios. Orra handles the complexity
while you focus on building your application.

## Next Steps

### 1. Integrate Services & Agents

#### Using Javascript

```javascript
import { initAgent } from '@orra.dev/sdk';

const myAgent = initAgent({
	name: 'ai-agent',
	orraUrl: process.env.ORRA_URL,
	orraKey: process.env.ORRA_API_KEY
});

// Turn your existing AI Agent into an orchestrated component
await myAgent.register({/*...*/ });
myAgent.start(async (task) => {/*...*/
});
```

→ [JS SDK Integration Guide](docs/sdks/js-sdk.md)

#### Using Python

```python
import os
from orra import OrraAgent, Task


class InputModel(BaseModel):
    customer_id: str
    message: str


class OutputModel(BaseModel):
    response: str


async def main():
    # Turn your existing AI Agent into an orchestrated component
    agent = OrraAgent(
        name="agent-name",
        description="What this agent does",
        url=os.getenv(ORRA_URL),
        api_key=os.getenv(ORRA_API_KEY)
    )

    @agent.handler()
    async def handle_request(task: Task[InputModel]) -> OutputModel:
        # Handler implementation that wraps your existing Agent or crew
        # built with your framework of choice.
        pass
```

→ [Python SDK Integration Guide](docs/sdks/python-sdk.md)

### 2. Orchestrate Actions

```shell
orra verify run "Estimate delivery for customer order" \
  -d customerId:CUST123 \
  -d orderId:ORD123
```

→ [Action Orchestration Guide](docs/actions.md)

### 3. Explore Examples

- 🛒 [E-commerce AI Assistant](examples/ecommerce-agent-app) - E-commerce customer service with a delivery specialised
  agent
- 👻 [Ghostwriters](examples/crewai-ghostwriters) - Content generation example showcasing how to use Orra
  with [CrewAI](https://www.crewai.com)
- 📣 [Echo Service (JavaScript)](examples/echo-js) - Simple example showing core concepts using JS
- 📣 [Echo Service (Python)](examples/echo-python) - Simple example showing core concepts using Python

### 4. Explore Docs and Guides

- [Rapid Multi-Agent App Development with Orra](docs/rapid-agent-app-devlopment.md)
- [Orchestrating Actions with Orra](docs/actions.md)
- [Advanced Topics & Internals](docs/advanced.md)

## Alpha Features & Limitations

### Available Now

* Robust Compensation guardrails for revertible services and agents 🆕🎉
* Prototype your multi-agent app in a single file then split and deploy as containers 🆕🎉
* LLM-powered task decomposition and routing
* In-memory execution tracking with exactly-once guarantees
* Smart service health handling with execution pausing and heartbeat monitoring
* Short-term retries with exponential backoff (up to 5 attempts)
* Intuitive JavaScript and Python SDKs
* CLI for Orra-powered projects management
* Automatic parallel execution optimization
* Built-in service discovery

### Current Limitations

1. **Storage**: All state is in-memory and will be lost on control plane restart
2. **Deployment**: Single-instance only, designed for development and self-hosted deployments

### Coming Soon

* Ahead of time execution plan domain grounding with pre-execution checks
* Agent replay and multi-LLM consensus planning
* Continuous adjustment of Agent workflows during runtime
* Additional language SDKs - Ruby, DotNet and Go very soon!
  ... and many more planned

## Join Our Alpha Testing Community

**We're looking for developers who:**

- Are building multi-agent applications
- Want to help shape Orra's development
- Are comfortable working with Alpha software
- Can provide feedback on real-world use cases

**Connect With Us:**

- [GitHub Discussions](https://github.com/orra-dev/orra/discussions) - Share your experience and ideas
- Office Hours - Weekly calls with the team

## License

Orra is MPL-2.0 licensed.
