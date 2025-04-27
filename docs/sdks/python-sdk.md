# Orra Python SDK Documentation

The Python SDK for orra lets you transform your AI agents, tools as services and services into reliable, production-ready components.

## Installation

First, [install the orra CLI](../cli.md).

Then, install the latest version of the SDK:
```bash
pip install orra-sdk
```

## Quick Integration Example

The orra SDK is designed to wrap your existing service logic with minimal changes. Here's a simple example showing how to integrate an existing chat service:

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
    # Initialize the orra service
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
            # and orra reacts accordingly.
            response = await my_service(task.input.customer_id, task.input.message)
            
            # Send final progress update
            await task.push_update({
                "progress": 100,
                "message": "Completed response generation"
            })
            
            return ChatOutput(response=response)
        except Exception as e:
            # Once you determine the task should fail, throw the error.
            # orra will handle failure propagation to the Plan Engine.
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

The orra SDK follows patterns similar to serverless functions or job processors, making it familiar for AI Engineers. Your services become event-driven handlers that:

1. Register capabilities with orra's Plan Engine (what they can do)
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
        
        # 2. Your existing business logic, may include its own retry/recovery if available (otherwise orra deals with this)
        result = await your_existing_function(input_data)
        
        # 3. Return results
        return OutputModel(**result)
    except Exception as e:
        # After your error handling is complete, let orra know about permanent failures
        raise
```

### 3. Aborting Tasks

You can abort a task's execution when you need to stop processing but don't want to trigger a normal failure. This is useful for business logic conditions where it makes sense to halt the orchestration with a specific message:

```python
from orra import OrraService, Task
from pydantic import BaseModel

class InventoryInput(BaseModel):
    product_id: str
    quantity: int

class InventoryOutput(BaseModel):
    success: bool
    reservation_id: str

@service.handler()
async def reserve_inventory(task: Task[InventoryInput]) -> InventoryOutput:
    # Check inventory availability
    inventory_count = await check_inventory(task.input.product_id)
    
    # If inventory is insufficient, abort the task
    if inventory_count < task.input.quantity:
        await task.abort({
            "reason": "INSUFFICIENT_INVENTORY",
            "available": inventory_count,
            "requested": task.input.quantity,
            "product_id": task.input.product_id
        })
        # Code below won't execute due to abort
    
    # Normal processing continues if not aborted
    reservation_id = await create_reservation(task.input.product_id, task.input.quantity)
    
    return InventoryOutput(success=True, reservation_id=reservation_id)
```

When a task is aborted:
- The orchestration stops execution
- The abort payload is preserved
- If any previously executed services are revertible, their compensation will be triggered
- The abort information is available in the compensation context

### 4. Reverts powered by compensations

Marking a service or agent as **revertible** enables the previous task result to be compensated by that component in case of upstream failures or aborts.

A revert may succeed **completely**, **partially** or simply **fail**. They are run after an action's failure.

Checkout the [Compensations](../compensations.md) doc for full explanation.

```python
from orra import OrraService, Task, CompensationResult, CompensationStatus, RevertSource
from pydantic import BaseModel
from typing import Optional, Dict, Any

class InventoryInput(BaseModel):
    product_id: str
    quantity: int

class InventoryOutput(BaseModel):
    success: bool
    reservation_id: str

service = OrraService(
   name="inventory-service",
   description="Manages product inventory",
   url="https://api.orra.dev",
   api_key="sk-orra-...",
   revertible=True,
)

@service.revert_handler()
async def revert_reservation(source: RevertSource[InventoryInput, InventoryOutput]) -> CompensationResult:
    # Access the original task input and result
    print(f"Reverting for product: {source.input.product_id}")
    print(f"Reservation ID to release: {source.output.reservation_id}")
    
    # Access compensation context information
    if source.context:
        # Why the compensation was triggered: "orchestration_failed", "aborted", etc.
        print(f"Compensation reason: {source.context.reason}")
        
        # ID of the parent orchestration
        print(f"Orchestration ID: {source.context.orchestration_id}")
        
        # When the compensation was initiated
        print(f"Timestamp: {source.context.timestamp}")
        
        # Access abort payload if task was aborted
        if source.context.reason == "aborted" and source.context.payload:
            print(f"Abort reason: {source.context.payload.get('reason')}")
            print(f"Available inventory: {source.context.payload.get('available')}")
            print(f"Requested quantity: {source.context.payload.get('requested')}")
            
            # For inventory-specific aborts, we might not need to do anything
            if (source.context.payload.get('reason') == "INSUFFICIENT_INVENTORY" and 
                source.context.payload.get('product_id') == source.input.product_id):
                print('No actual reservation was made due to insufficient inventory')
                return CompensationResult(status=CompensationStatus.COMPLETED)
    
    # For other cases where a reservation was made, release it
    if source.output and source.output.reservation_id:
        try:
            await release_reservation(source.output.reservation_id)
            print(f"Released reservation {source.output.reservation_id}")
            return CompensationResult(status=CompensationStatus.COMPLETED)
        except Exception as e:
            print(f"Failed to release reservation: {str(e)}")
            return CompensationResult(
                status=CompensationStatus.FAILED,
                error=str(e)
            )
    else:
        # No reservation ID found
        print('No reservation to revert')
        return CompensationResult(status=CompensationStatus.COMPLETED)
```

#### Compensation Context

The compensation context provides critical information about why a compensation was triggered:

| Property | Description                                                                                            |
|----------|--------------------------------------------------------------------------------------------------------|
| `reason` | Why the compensation was triggered during an orchestration (e.g., `"failed"`, `"aborted"`)             |
| `orchestrationId` | The ID of the parent orchestration                                                                     |
| `timestamp` | When the compensation was initiated                                                                    |
| `payload` | **Optional**: additional data related to the compensation (may contain aborted or failed payload) |

This context helps you implement more intelligent compensation logic based on the specific reason for the rollback.

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

Use the orra CLI to view progress updates:

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

orra's Plan Engine maintains service/agent identity across restarts using persistence. This is crucial for:
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
    - Use `task.abort()` for business-logic conditions that should stop execution

2. **Schema Design**
    - Use Pydantic models for type safety
    - Include comprehensive field descriptions
    - Keep schemas focused and minimal

3. **Service Design**
    - Keep services focused on specific capabilities
    - Design for idempotency
    - Include proper logging for debugging

4. **Compensation Design**
    - Make compensation logic robust with its own error handling
    - Use the compensation context to implement context-aware reverts
    - Test compensation scenarios thoroughly

## Example: Converting Existing Code

Here's how to convert an existing AI service to use orra:

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

### After (orra Integration)
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

@agent.handler()
async def handle_analysis(task: Task[ImageInput]) -> ImageOutput:
    try:
        # Check if the image is accessible
        is_accessible = await check_image_access(task.input.image_url)
        if not is_accessible:
            await task.abort({
                "reason": "IMAGE_NOT_ACCESSIBLE",
                "url": task.input.image_url
            })
            # Code below won't execute due to abort
        
        # Your function handles its own retries
        result = await analyze_image(task.input.image_url)
        return ImageOutput(**result)
    except Exception as e:
        # After your error handling is complete, let orra know about the failure
        raise
```

## Complete Example: Revertible Service with Abort and Context

Here's a complete example of an inventory service that demonstrates abort functionality and context-aware compensation:

```python
import asyncio
from orra import OrraService, Task, CompensationResult, CompensationStatus, RevertSource
from pydantic import BaseModel
from inventory_db import check_availability, create_reservation, release_reservation

class InventoryInput(BaseModel):
    order_id: str
    product_id: str
    quantity: int

class InventoryOutput(BaseModel):
    success: bool
    reservation_id: str

async def main():
    # Initialize service
    inventory_service = OrraService(
        name="inventory-service",
        description="Handles product inventory reservations",
        url="https://api.orra.dev",
        api_key="sk-orra-...",
        revertible=True  # Enable compensations
    )

    # Main task handler
    @inventory_service.handler()
    async def reserve_inventory(task: Task[InventoryInput]) -> InventoryOutput:
        try:
            # Send progress update
            await task.push_update({
                "progress": 25,
                "status": "checking_inventory",
                "message": "Checking product availability"
            })
            
            # Check inventory
            available = await check_availability(task.input.product_id)
            
            if available < task.input.quantity:
                # Abort the task with detailed information
                await task.abort({
                    "reason": "INSUFFICIENT_INVENTORY",
                    "available": available,
                    "requested": task.input.quantity,
                    "product_id": task.input.product_id
                })
                # Code below won't execute due to abort
            
            # Update progress
            await task.push_update({
                "progress": 75,
                "status": "reserving",
                "message": "Reserving inventory"
            })
            
            # Reserve inventory
            reservation_id = await create_reservation(
                task.input.order_id,
                task.input.product_id,
                task.input.quantity
            )
            
            # Final progress update
            await task.push_update({
                "progress": 100,
                "status": "completed",
                "message": "Inventory reserved successfully"
            })
            
            return InventoryOutput(
                success=True,
                reservation_id=reservation_id
            )
        except Exception as e:
            # Handle errors
            raise

    # Compensation handler with context awareness
    @inventory_service.revert_handler()
    async def revert_reservation(source: RevertSource[InventoryInput, InventoryOutput]) -> CompensationResult:
        print(f"Reverting reservation for order {source.input.order_id}")
        
        # Use context to determine compensation behavior
        if source.context:
            print(f"Compensation triggered by: {source.context.reason}")
            print(f"Orchestration ID: {source.context.orchestration_id}")
            
            # Log additional information for aborted tasks
            if source.context.reason == "aborted" and source.context.payload:
                print(f"Abort reason: {source.context.payload.get('reason')}")
                
                # For inventory-specific aborts, we might not need to do anything
                if (source.context.payload.get('reason') == "INSUFFICIENT_INVENTORY" and 
                    source.context.payload.get('product_id') == source.input.product_id):
                    print('No actual reservation was made due to insufficient inventory')
                    return CompensationResult(status=CompensationStatus.COMPLETED)
        
        # For other cases where a reservation was made, release it
        if source.output and source.output.reservation_id:
            try:
                await release_reservation(source.output.reservation_id)
                print(f"Released reservation {source.output.reservation_id}")
                return CompensationResult(status=CompensationStatus.COMPLETED)
            except Exception as e:
                print(f"Failed to release reservation: {str(e)}")
                return CompensationResult(
                    status=CompensationStatus.FAILED,
                    error=str(e)
                )
        else:
            # No reservation ID found
            print('No reservation to revert')
            return CompensationResult(status=CompensationStatus.COMPLETED)

    # Start the service
    await inventory_service.start()

    try:
        # Keep the service running
        await asyncio.get_running_loop().create_future()
    except KeyboardInterrupt:
        await inventory_service.shutdown()

if __name__ == "__main__":
    asyncio.run(main())
```

## Next Steps

1. Review the example projects for more integration patterns
2. Join our community for support and updates
3. Check out the action orchestration guide to start using your services

Need help? Contact the orra team or open an issue on GitHub.
