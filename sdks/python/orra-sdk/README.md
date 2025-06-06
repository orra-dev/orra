# Orra SDK for Python

Python SDK for [Orra](https://github.com/orra-dev/orra) - Build reliable multi-agent applications that handle complex real-world interactions.

## Installation

```bash
pip install orra-sdk
```

## Usage

```python
import asyncio
from orra import OrraService, Task
from pydantic import BaseModel

# Define your models
class Input(BaseModel):
    message: str

class Output(BaseModel):
    response: str

# Initialize the SDK
echo_service = OrraService(
    name="echo",
    description="A simple echo provider that echoes whatever its sent",
    url="https://api.orra.dev",
    api_key="your-api-key"
)

# Register your service handler
@echo_service.handler()
async def handle_message(task: Task[Input]) -> Output:
    return Output(response=f"Echo: {task.input.message}")

# Run the service
async def main():
    try:
        await echo_service.start()
    except KeyboardInterrupt:
        await echo_service.shutdown()

if __name__ == "__main__":
    asyncio.run(main())
```

## Advanced Features

### Revertible Services with Compensations

```python
from orra import OrraService, Task, CompensationResult, CompensationStatus, RevertSource

# Define your models
class Input(BaseModel):
    order_id: str
    product_id: str
    quantity: int

class Output(BaseModel):
    success: bool
    reservation_id: str

# Initialize as revertible
inventory_service = OrraService(
    name="inventory-service",
    description="Handles product inventory reservations",
    url="https://api.orra.dev",
    api_key="your-api-key",
    revertible=True
)

# Main handler
@inventory_service.handler()
async def reserve_inventory(task: Task[Input]) -> Output:
    # Reserve inventory logic
    return Output(success=True, reservation_id="res-123456")

# Compensation handler for reverting
@inventory_service.revert_handler()
async def revert_reservation(source: RevertSource[Input, Output]) -> CompensationResult:
    # Access the compensation context information
    if source.context:
        reason = source.context.reason  # Why compensation was triggered (ABORTED, FAILED, etc)
        payload = source.context.payload  # Any additional data passed with compensation
        orchestration_id = source.context.orchestration_id  # Parent orchestration ID

        print(f"Compensation triggered for orchestration {orchestration_id} due to: {reason}")
        if payload:
            print(f"Additional context: {payload}")

    print(f"Reverting reservation {source.output.reservation_id} for order {source.input.order_id}")
    # Release the inventory
    return CompensationResult(status=CompensationStatus.COMPLETED)
```

### Aborting Tasks

```python
from orra import OrraService, Task
from pydantic import BaseModel

class Input(BaseModel):
    order_id: str
    product_id: str
    quantity: int

class Output(BaseModel):
    success: bool
    reservation_id: str

inventory_service = OrraService(
    name="inventory-service",
    description="Handles product inventory reservations",
    url="https://api.orra.dev",
    api_key="your-api-key",
    revertible=True  # Enable compensation for aborted tasks
)

@inventory_service.handler()
async def reserve_inventory(task: Task[Input]) -> Output:
    # Check inventory availability
    inventory_count = await check_inventory(task.input.product_id)
    
    if inventory_count < task.input.quantity:
        # If inventory is insufficient, abort the task
        await task.abort({
            "reason": "INSUFFICIENT_INVENTORY",
            "available": inventory_count,
            "requested": task.input.quantity
        })
        # Code below won't execute due to abort
    
    # Normal processing continues if not aborted
    reservation_id = await create_reservation(
        task.input.order_id,
        task.input.product_id,
        task.input.quantity
    )
    
    return Output(success=True, reservation_id=reservation_id)
```

Aborting a task immediately stops execution and sends an abort signal to the orchestration engine. If the service is revertible, the abort information will be available in the compensation context:

```python
@inventory_service.revert_handler()
async def revert_reservation(source: RevertSource[Input, Output]) -> CompensationResult:
    if source.context and source.context.reason == "aborted":
        # The abort payload is available in context.payload
        abort_info = source.context.payload
        print(f"Task was aborted due to: {abort_info.get('reason')}")
        print(f"Available inventory: {abort_info.get('available')}")
        print(f"Requested quantity: {abort_info.get('requested')}")
    
    # No actual reservation to revert in this case
    return CompensationResult(status=CompensationStatus.COMPLETED)
```

You can view abort information using the CLI:
```bash
# View task details including abort reason
orra inspect -d <orchestration-id> -t <task-id>
```

### Progress Updates for Long-Running Tasks

```python
from orra import OrraService, Task
from pydantic import BaseModel

class Input(BaseModel):
    file_path: str

class Output(BaseModel):
    success: bool
    processed_rows: int

service = OrraService(
    name="data-processor",
    description="Processes large data files",
    url="https://api.orra.dev",
    api_key="your-api-key"
)

@service.handler()
async def process_file(task: Task[Input]) -> Output:
    try:
        # Start processing
        await task.push_update({
            "progress": 20,
            "status": "processing",
            "message": "Starting file analysis"
        })
        
        # Perform initial processing
        await analyze_file(task.input.file_path)
        
        # Update progress halfway
        await task.push_update({
            "progress": 50,
            "status": "processing",
            "message": "Processing data rows"
        })
        
        # Complete processing
        rows = await process_data(task.input.file_path)
        
        # Final progress update
        await task.push_update({
            "progress": 100,
            "message": "Processing complete"
        })
        
        return Output(success=True, processed_rows=rows)
    except Exception as e:
        # Handle errors appropriately
        raise
```

Progress updates allow you to send interim results to provide visibility into long-running tasks. View these updates using the CLI:

```bash
# View summarized updates (first/last)
orra inspect -d <orchestration-id> --updates

# View all detailed updates
orra inspect -d <orchestration-id> --long-updates
```

### Custom Persistence

```python
from pathlib import Path
from typing import Optional

def save_to_db(service_id: str) -> None:
    # Your database save logic
    pass

def load_from_db() -> Optional[str]:
    # Your database load logic
    return "previously-registered-service-id"

service = OrraService(
    name="my-service",
    description="Service with custom persistence",
    url="https://api.orra.dev",
    api_key="your-api-key",
    
    # File-based persistence with custom path
    persistence_method="file",
    persistence_file_path=Path("./custom/path/service-key.json"),
    
    # Or database persistence
    # persistence_method="custom",
    # custom_save=save_to_db,
    # custom_load=load_from_db
)
```

## Working with Agents

For AI agents instead of simple services:

```python
from orra import OrraAgent, Task
from pydantic import BaseModel

class AgentInput(BaseModel):
    query: str
    context: str

class AgentOutput(BaseModel):
    response: str
    confidence: float

agent = OrraAgent(
    name="qa-agent",
    description="Question answering agent with context",
    url="https://api.orra.dev",
    api_key="your-api-key"
)

@agent.handler()
async def handle_question(task: Task[AgentInput]) -> AgentOutput:
    # Agent processing logic here
    return AgentOutput(
        response="This is the answer to your question.",
        confidence=0.95
    )
```

## Documentation

For more detailed documentation, please visit [Orra Python SDK Documentation](https://github.com/orra-dev/orra/blob/main/docs/sdks/python-sdk.md).
