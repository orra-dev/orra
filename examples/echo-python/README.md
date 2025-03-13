# Echo Tool as a Service Example (Python)

A minimal example demonstrating how to build and coordinate a tool as a service using Orra's Plan Engine. It's Orra Hello World!

```mermaid
sequenceDiagram
    participant CLI as Orra CLI
    participant PE as Plan Engine
    participant ES as Echo Tool as Service
    participant WH as Webhook Server

    CLI->>PE: Send action
    PE->>ES: Orchestrate task
    ES->>PE: Return echo
    PE->>WH: Send result
    Note over WH: See result in terminal
```

## ✨ Features

- 🔄 Basic service registration and coordination
- 📡 Real-time WebSocket communication
- ⚡ Reliable message delivery
- 🛡️ Built-in health monitoring
- 🚀 Simple but production-ready patterns

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and [Docker Compose](https://docs.docker.com/compose/install/)
- [Poetry](https://python-poetry.org/docs/#installation)
- [OpenAI API key](https://platform.openai.com/api-keys) for Orra's Plan Engine `PLAN_CACHE_OPENAI_API_KEY`
- [OpenAI API key](https://platform.openai.com/api-keys) or [Groq API key](https://console.groq.com/docs/quickstart) for Orra's Plan Engine reasoning models config
- [OpenAI API key](https://platform.openai.com/api-keys) for the `writer_crew` and `editor` Agents

## Setup

1. First, setup Orra and the CLI by following the [installation instructions](../../README.md#installation):

2. Setup your Orra project:
```bash
# Create project, add a webhook and API key
orra projects add my-echo-app
orra webhooks add http://host.docker.internal:8888/webhook
orra api-keys gen echo-key
```

3. Configure the Echo tool as service
```bash
cd examples/echo-python
echo "ORRA_API_KEY=echo-key-from-step-2" > .env
```

## Running the Example

1. Start the webhook server (in a separate terminal):
```bash
# Start the webhook server using the verify subcommand
orra verify webhooks start http://localhost:8888/webhook
```

2. Start and register the Echo service:
```bash
# With Docker
docker compose up

# Or locally with Poetry
poetry install
poetry run python src/main.py
```

3. Try it out:
```bash
# Send a test message
orra verify run 'Echo this message' --data message:'Hello from Orra!'

# Check the result
orra ps
orra inspect <orchestration-id>
```

You should see the result both in the webhook server terminal and through the inspect command.

```bash
# This curl command is equivalent to orra verify run performs internally  
## Send an echo orchestration request to the orra Plan Engine

curl -X POST http://localhost:8005/orchestrations \
  -H "Authorization: Bearer $ORRA_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "action": {
      "type": "echo",
      "content": "Echo this message"
    },
    "data": [
      {
        "field": "message",
        "value": "Hello from curl!"
      }
    ],
    "webhook": "http://host.docker.internal:8888/api/webhook"
  }'
```

## SDK Integration Example

The core Echo service implementation is straightforward:

```python
from orra import OrraService, Task
from pydantic import BaseModel

class EchoInput(BaseModel):
    message: str

class EchoOutput(BaseModel):
    echo: str

service = OrraService(
    name="echo",
    description="Use echo to echo back messages",
    url=os.getenv("ORRA_URL"),
    api_key=os.getenv("ORRA_API_KEY")
)

@service.handler()
async def handle_echo(task: Task[EchoInput]) -> EchoOutput:
    return EchoOutput(echo=task.input.message)
```

That's it! Orra provides:
- Service discovery
- Health monitoring
- Reliable task execution
- Error recovery

## Learn More

- [Orra Documentation](../../docs)
- [CLI Documentation](../../docs/cli.md)
