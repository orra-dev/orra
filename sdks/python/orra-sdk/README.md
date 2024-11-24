# Orra SDK for Python

Python SDK for building reliable multi-agent applications with Orra.

## Installation

```bash
pip install orra-sdk
```

## Usage

```python
from orra import OrraSDK
from pydantic import BaseModel

# Initialize the SDK
orra = OrraSDK(
    url="https://api.orra.dev",
    api_key="your-api-key"
)

# Define your models
class Input(BaseModel):
    message: str

class Output(BaseModel):
    response: str

# Register your service
@orra.service(
    name="my-service",
    description="A simple echo service",
    input_model=Input,
    output_model=Output
)
async def handle_message(request: Input) -> Output:
    return Output(response=f"Echo: {request.message}")

# Run the service
if __name__ == "__main__":
    orra.run()
```
