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
    print(f"Reverting reservation {source.output.reservation_id} for order {source.input.order_id}")
    # Release the inventory
    return CompensationResult(status=CompensationStatus.COMPLETED)
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
