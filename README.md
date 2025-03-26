# 🪡 orra

Move beyond simple Crews and Agents. Use orra to build production-ready multi-agent applications that handle complex real-world interactions.

![](images/orra-diagram.png)

orra coordinates tasks across your existing stack, agents and any tools run as services using intelligent reasoning — across any language, agent framework or deployment platform.

* 🧠 Smart pre-evaluated execution plans
* 🎯 Domain grounded
* 🗿 Durable execution
* 🚀 Go fast with tools as services
* ↩️ Revert state to handle failures
* ⛑️ Automatic service health monitoring
* 🔮 Real-time status tracking
* 🪝 Webhook result delivery

[Learn why we built orra →](https://tinyurl.com/orra-launch-blog-post)

### Coming Soon

* Agent replay and multi-LLM consensus planning
* Continuous adjustment of Agent workflows during runtime
* Additional language SDKs - Ruby, DotNet and Go very soon!
* MCP integration
* Leverage Local LLMs

## Table of Contents

- [Installation](#installation)
- [How The Plan Engine Works](#how-the-plan-engine-works)
- [Guides](#guides)
- [Explore Examples](#explore-examples)
- [Docs](#docs)
- [Self Hosting](#self-hosting)
- [Support](#support)
- [License](#license)

## Installation

### Prerequisites

- [Docker](https://docs.docker.com/desktop/) and [Docker Compose](https://docs.docker.com/compose/install/) - For running the Plan Engine
- Set up Reasoning and Embedding Models to power task planning and execution plan caching/validation

#### Setup Reasoning Models

Select between Groq's [deepseek-r1-distill-llama-70b](https://groq.com/groqcloud-makes-deepseek-r1-distill-llama-70b-available/) model or OpenAI's [o1-mini / o3-mini](https://platform.openai.com/docs/guides/reasoning) models.

Update the .env file with one of these:

**Groq**
```shell
# GROQ Reasoning
REASONING_PROVIDER=groq
REASONING_MODEL=deepseek-r1-distill-llama-70b
REASONING_API_KEY=xxxx
```

**O1-mini**
```shell
# OpenAI Reasoning
REASONING_PROVIDER=openai
REASONING_MODEL=o1-mini
REASONING_API_KEY=xxxx
```

**O3-mini**
```shell
# OpenAI Reasoning
REASONING_PROVIDER=openai
REASONING_MODEL=o3-mini
REASONING_API_KEY=xxxx
```

#### Setup Embedding Models

Update the .env file with:
```shell
# Execution Plan Cache and validation OPENAI API KEY
PLAN_CACHE_OPENAI_API_KEY=xxxx
```

### 1. Install orra CLI

Download the latest CLI binary for your platform from our [releases page](https://github.com/orra-dev/orra/releases):

```shell
# macOS
curl -L https://github.com/orra-dev/orra/releases/download/v0.2.3/orra-darwin-arm64 -o /usr/local/bin/orra
chmod +x /usr/local/bin/orra

# Linux
curl -L https://github.com/ezodude/orra/releases/download/v0.2.3/orra-linux-amd64 -o /usr/local/bin/orra
chmod +x /usr/local/bin/orra

# Verify installation
orra version
```

→ [Full CLI documentation](docs/cli.md)

### 2. Get orra Plan Engine Running

Clone the repository and start the Plan Engine:

```shell
git clone https://github.com/ezodude/orra.git
cd orra/planengine

# Start the Plan Engine
docker compose up --build
```

## How The Plan Engine Works

The Plan Engine powers your multi-agent applications through intelligent planning and reliable execution:

### Progressive Planning Levels

#### 1. Base Planning

Your agents stay clean and simple (wrapped in the orra SDK):

**Python**
```python
from orra import OrraAgent, Task
from pydantic import BaseModel

class ResearchInput(BaseModel):
    topic: str
    depth: str

class ResearchOutput(BaseModel):
    summary: str

agent = OrraAgent(
    name="research-agent",
    description="Researches topics using web search and knowledge base",
    url="https://api.orra.dev",
    api_key="sk-orra-..."
)

@agent.handler()
async def research(task: Task[ResearchInput]) -> ResearchOutput:
    results = await run_research(task.input.topic, task.input.depth)
    return ResearchOutput(summary=results.summary)
```

**JavaScript**
```javascript
import { initAgent } from '@orra.dev/sdk';

const agent = initAgent({
  name: 'research-agent',
  orraUrl: process.env.ORRA_URL,  
  orraKey: process.env.ORRA_API_KEY
});

await agent.register({
  description: 'Researches topics using web search and knowledge base',
  schema: {
    input: {
      type: 'object',
      properties: {
        topic: { type: 'string' },
        depth: { type: 'string' }
      }
    },
    output: {
      type: 'object',
      properties: {
        summary: { type: 'string' }
      }
    }
  }
});

agent.start(async (task) => {
  const results = await runResearch(task.input.topic, task.input.depth);
  return { summary: results.summary };
});
```

Features:
* AI analyzes intent and creates execution plans that target your components
* Automatic service discovery and coordination
* Parallel execution where possible

#### 2. Production Planning with Domain Grounding

```yaml
# Define domain constraints
name: research-workflow
domain: content-generation
use-cases:
  - action: "Research topic {topic}"
    capabilities: 
      - "Web search access"
      - "Knowledge synthesis"
constraints:
  - "Verify sources before synthesis"
  - "Maximum research time: 10 minutes"
```

Features:
* Full semantic validation of execution plans
* Capability matching and verification
* Safety constraints enforcement
* State transition validation

#### 3. Reliable Execution

```bash
# Execute an action with the Plan Engine
orra verify run "Research and summarize AI trends" \
  --data topic:"AI in 2024" \
  --data depth:"comprehensive"
```

The Plan Engine ensures:
* Automatic service health monitoring
* Stateful execution tracking
* Built-in retries and recovery
* Real-time status updates
* Webhook result delivery

## Guides

- [From Fragile to Production-Ready Multi-Agent App](https://github.com/orra-dev/agent-fragile-to-prod-guide)

## Explore Examples

- 🛒 [E-commerce AI Assistant (JavaScript)](examples/ecommerce-agent-app) - E-commerce customer service with a delivery specialized agent
- 👻 [Ghostwriters (Python)](examples/crewai-ghostwriters) - Content generation example showcasing how to use orra with [CrewAI](https://www.crewai.com) 🆕🎉
- 📣 [Echo Tools as Service (JavaScript)](examples/echo-js) - Simple example showing core concepts using JS
- 📣 [Echo Tools as Service (Python)](examples/echo-python) - Simple example showing core concepts using Python

## Docs

- [Rapid Multi-Agent App Development with orra](docs/rapid-agent-app-devlopment.md)
- [What is an Agent in orra?](docs/what-is-agent.md)
- [Orchestrating Actions with orra](docs/actions.md)
- [Domain Grounding Execution](docs/grounding.md)
- [Execution Plan Caching](docs/plan-caching.md)
- [Core Topics & Internals](docs/core.md)

## Self Hosting

The orra Plan Engine is packaged with a [Dockerfile](planengine/Dockerfile). Run it a single instance locally using docker or docker compose.

**For production**, run the Plan Engine as single instance on a Cloud Service like [Digital Ocean](https://docs.digitalocean.com/products/app-platform/how-to/deploy-from-monorepo/) by pointing to the `planengine` folder during setup. 

The Plan Engine uses the [BadgerDB](https://github.com/hypermodeinc/badger) embedded database to persist all state - operational information is queryable using the [orra CLI](docs/cli.md).  

[Book an office hours slot](https://cal.com/orra-dev/office-hours), to get help hosting or running orra's Plan Engine for production.

## Support

Need help? We're here to support you:

- Report a bug or request a feature by creating an [issue](https://github.com/orra-dev/orra/issues/new?template=bug-report-feature-request.yml)
- Start a [discussion](https://github.com/orra-dev/orra/discussions) about your ideas or questions

## License

Orra is MPL-2.0 licensed.
