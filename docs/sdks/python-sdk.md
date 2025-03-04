# Orra Python SDK Documentation

The Python SDK for Orra lets you transform your AI agents, tools as services and services into reliable, production-ready components.

## Installation

First, [install the Orra CLI](../cli.md).

Then, install the latest version of the SDK:
```bash
pip install orra-sdk
```

## Quick Integration Example

The Orra SDK is designed to wrap your existing service logic with minimal changes. Here's a simple example showing how to integrate an existing chat service:

```python
import asyncio
from orra import OrraService, Task
from pydantic import BaseModel
from existing_service import my_service  # Your existing logic

# Define your models
class ChatInput(BaseModel):
    customer_id: str
    message: str

class ChatOutput(BaseModel):
    response: str

async def main():
    # Initialize the Orra service
    cust_chat_svc = OrraService(
        name="customer-chat-service",
        description="Handles customer chat interactions",
        url="https://api.orra.dev",
        api_key="sk-orra-..."
    )

    # Register your handler
    @cust_chat_svc.handler()
    async def handle_chat(task: Task[ChatInput]) -> ChatOutput:
        try:
            # Send progress update
            await task.push_update({
                "progress": 25,
                "message": "Processing customer request..."
            })
            
            # Use your existing service function
            # Your function handles its own retries and error recovery
            # and Orra reacts accordingly.
            response = await my_service(task.input.customer_id, task.input.message)
            
            # Send final progress update
            await task.push_update({
                "progress": 100,
                "message": "Completed response generation"
            })
            
            return ChatOutput(response=response)
        except Exception as e:
            # Once you determine the task should fail, throw the error.
            # Orra will handle failure propagation to the control plane.
            raise
    
    await asyncio.gather(cust_chat_svc.start())

    try:
        await asyncio.get_running_loop().create_future()
    except KeyboardInterrupt:
        await asyncio.gather( cust_chat_svc.shutdown() )

if __name__ == "__main__":
    asyncio.run(main())
```

## Understanding the SDK

The Orra SDK follows patterns similar to serverless functions or job processors, making it familiar for AI Engineers. Your services become event-driven handlers that:

1. Register capabilities with Orra's Plan Engine (what they can do)
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

```python
# For stateless services
from orra import OrraService, Task

service = OrraService(
    name="service-name",
    description="What this service does",
    url="https://api.orra.dev",
    api_key="sk-orra-..."
)

class InputModel(BaseModel):
    customer_id: str
    message: str

class OutputModel(BaseModel):
    response: str

@service.handler()
async def handle_request(task: Task[InputModel]) -> OutputModel:
    # Handler implementation
    pass

# For AI agents
from orra import OrraAgent

agent = OrraAgent(
    name="agent-name",
    description="What this agent does",
    url="https://api.orra.dev",
    api_key="sk-orra-..."
)

@agent.handler()
async def handle_request(task: Task[InputModel]) -> OutputModel:
    # Handler implementation
    pass
```

### 2. Task Handler Implementation

Implement your task handler to process requests:

```python
class InputModel(BaseModel):
    customer_id: str
    message: str

class OutputModel(BaseModel):
    response: str

@service.handler()
async def handle_request(task: Task[InputModel]) -> OutputModel:
    try:
        # 1. Access task information
        input_data = task.input
        
        # 2. Your existing business logic, may include its own retry/recovery if available (otherwise Orra deals with this)
        result = await your_existing_function(input_data)
        
        # 3. Return results
        return OutputModel(**result)
    except Exception as e:
        # After your error handling is complete, let Orra know about permanent failures
        raise
```

### 3. Reverts powered by compensations

Marking a service or agent as **revertible** enables the previous task result to be compensated by that component in case of upstream failures.

A revert may succeed **completely**, **partially** or simply **fail**. They are run after an action's failure.

Checkout the [Compensations](../compensations.md) doc for full explanation.

```python
service = OrraService(
   name="service-name",
   description="What this service does",
   url="https://api.orra.dev",
   api_key="sk-orra-...",
   revertible=True,
)

@service.revert_handler()
async def revert_stuff(source: RevertSource[InputModel, OutputModel]) -> CompensationResult:
   print(f"Reverting for customer: {source.input.customer_id}")
   print(f"Reverting response: {source.output.response}")
   return CompensationResult(status=CompensationStatus.COMPLETED)
```

## Advanced Features

### Progress Updates

For long-running tasks, you can send interim progress updates to the orchestration engine. This allows monitoring task execution in real-time and provides valuable information for debugging and audit logs.

The update is any object with properties that make sense for the task underway.

```python
@service.handler()
async def handle_request(task: Task[InputModel]) -> OutputModel:
    try:
        # 1. Begin processing
        await task.push_update({
            "progress": 20,
            "status": "processing",
            "message": "Starting data analysis"
        })
        
        # 2. Continue with more steps
        await some_function()
        await task.push_update({
            "progress": 50,
            "status": "processing",
            "message": "Processing halfway complete"
        })
        
        # 3. Almost done
        await final_steps()
        await task.push_update({
            "progress": 90,
            "status": "processing",
            "message": "Finalizing results"
        })
        
        # 4. Return final result
        return OutputModel(response="Analysis complete", data=[...])
    except Exception as e:
        # Handle errors
        raise
```

#### Benefits of Progress Updates

- **Visibility**: Track execution of long-running tasks in real-time
- **Debugging**: Identify exactly where tasks slow down or fail
- **Audit Trail**: Maintain a complete history of task execution
- **User Experience**: Forward progress information to end-users

#### Viewing Progress Updates

Use the Orra CLI to view progress updates:

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

Orra's Plan Engine maintains service/agent identity across restarts using persistence. This is crucial for:
- Maintaining service/agent history
- Ensuring consistent service/agent identification
- Supporting service/agent upgrades

```python
from pathlib import Path
from typing import Optional

def save_to_db(service_id: str) -> None:
    # Your db save logic
    pass

def load_from_db() -> Optional[str]:
    # Your db load logic
    pass

service = OrraService(
    name="a-service",
    url="https://api.orra.dev",
    api_key="sk-orra-...",
    # 1. File-based persistence (default)
    persistence_method="file",
    persistence_file_path=Path("./custom/path/service-key.json"),
    
    # 2. Custom persistence (e.g., database)
    persistence_method="custom",
    custom_save=save_to_db,
    custom_load=load_from_db
)
```

## Best Practices

1. **Error Handling**
    - Implement comprehensive error handling in your business logic
    - Use retries for transient failures

2. **Schema Design**
    - Use Pydantic models for type safety
    - Include comprehensive field descriptions
    - Keep schemas focused and minimal

3. **Service Design**
    - Keep services focused on specific capabilities
    - Design for idempotency
    - Include proper logging for debugging

## Example: Converting Existing Code

Here's how to convert an existing AI service to use Orra:

### Before (FastAPI Service)
```python
from fastapi import FastAPI
from ai_agent import analyze_image

app = FastAPI()

@app.post("/analyze")
async def analyze(image_url: str):
    try:
        analysis = await analyze_image(image_url)
        return analysis
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))
```

### After (Orra Integration)
```python
from orra import OrraAgent, Task
from pydantic import BaseModel
from ai_agent import analyze_image  # Reuse existing logic

class ImageInput(BaseModel):
    image_url: str

class ImageOutput(BaseModel):
    objects: list[str]
    labels: list[str]
    confidence: float

agent = OrraAgent(
    name="image-analysis-agent",
    description="Analyzes any image using AI",
    url="https://api.orra.dev",
    api_key="sk-orra-..."
)

async def main():
    @agent.handler()
    async def handle_analysis(task: Task[ImageInput]) -> ImageOutput:
        try:
            # Your function handles its own retries
            result = await analyze_image(task.input.image_url)
            return ImageOutput(**result)
        except Exception as e:
            # After your error handling is complete, let Orra know about the failure
            raise
```

## Next Steps

1. Review the example projects for more integration patterns
2. Join our community for support and updates
3. Check out the action orchestration guide to start using your services

Need help? Contact the Orra team or open an issue on GitHub.
