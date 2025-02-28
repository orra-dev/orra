#  This Source Code Form is subject to the terms of the Mozilla Public
#   License, v. 2.0. If a copy of the MPL was not distributed with this
#   file, You can obtain one at https://mozilla.org/MPL/2.0/.

"""
Echo Tool as a Service Example using Orra SDK

A simple echo tool as a service that demonstrates Plan Engine service coordination
and execution with proper lifecycle management and graceful shutdown.
"""

import os
import asyncio
from contextlib import asynccontextmanager
from concurrent.futures import ThreadPoolExecutor

import uvicorn
from dotenv import load_dotenv
from fastapi import FastAPI
from orra import OrraService, Task

from schema import EchoInput, EchoOutput
from config import get_persistence_config

# Load environment variables
load_dotenv()

# -------------- Service Setup --------------

async def create_orra_tool_service() -> OrraService:
    """Initialize and configure the Echo tool as a service"""
    tool_svc = OrraService(
        name="echo",
        description="An echo provider that echoes back messages",
        url=os.getenv("ORRA_URL", "http://localhost:8005"),
        api_key=os.getenv("ORRA_API_KEY"),
        log_level="DEBUG",
        **get_persistence_config()
    )

    @tool_svc.handler()
    async def handle_echo(task: Task[EchoInput]) -> EchoOutput:
        """Handle echo requests by returning the input message"""
        print(f"Echoing input: {task.input.message}")
        return EchoOutput(echo=task.input.message)

    return tool_svc

# -------------- FastAPI Lifecycle --------------

@asynccontextmanager
async def lifespan(app: FastAPI):
    """
    FastAPI lifespan context manager for service lifecycle management.
    Handles startup and shutdown of the Orra service.
    """
    # Create and start Orra service
    orra_tool_service = await create_orra_tool_service()
    await orra_tool_service.start()
    print("Echo started successfully")

    yield

    # Shutdown service
    print("Shutting down Echo...")
    await orra_tool_service.shutdown()

# Initialize FastAPI with lifespan
app = FastAPI(lifespan=lifespan)

@app.get("/health")
async def health_check():
    """Simple health check endpoint"""
    return {"status": "healthy"}

# -------------- Service Runner --------------

def run_fastapi():
    """Run the FastAPI server"""
    uvicorn.run(app, host="0.0.0.0", port=3500)

async def run_forever():
    """Keep the main task running"""
    while True:
        await asyncio.sleep(1)

async def start_services():
    """Start both FastAPI and Orra services"""
    with ThreadPoolExecutor() as executor:
        executor.submit(run_fastapi)
        await run_forever()

# -------------- Main Entry Point --------------

def main():
    """Main entry point with proper error handling"""
    try:
        print("Starting Echo...")
        asyncio.run(start_services())
    except KeyboardInterrupt:
        print("\nShutdown requested via keyboard interrupt")
    except Exception as e:
        print(f"Error running Echo: {e}")
        raise

if __name__ == "__main__":
    main()
