# Orra SDK for Python

Python SDK for building reliable multi-agent applications with Orra.

## Installation

```bash
pip install orra-sdk
```

## Usage

```python
from orra import OrraService, Task
from pydantic import BaseModel

# Initialize the SDK
svc = OrraService(
    name="my-service",
    description="A simple echo service",
    url="https://api.orra.dev",
    api_key="your-api-key"
)

# Define your models
class Input(BaseModel):
    message: str

class Output(BaseModel):
    response: str

# Register your service
@svc.handler()
async def handle_message(request: Task[Input]) -> Output:
    return Output(response=f"Echo: {request.input.message}")

# Run the service
if __name__ == "__main__":
    svc.start()
```
