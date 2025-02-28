# Rapid Multi-Agent App Development with Orra

Stop wrestling with rigid agent frameworks. Orra lets you build production-ready multi-agent systems the way you actually work - starting simple and scaling naturally.

## Why Orra?

As AI engineers, we've all been there:
- Struggling with inflexible agent frameworks that force specific patterns
- Hitting walls when moving from prototype to production
- Writing endless boilerplate for agent communication
- Fighting with fragile orchestration code
- Dealing with frameworks that assume all agents live in one process

Orra is different. It's built for AI engineers who need to:
1. **Prototype Fast**: Start with everything in one file - just like you would naturally
2. **Focus on Logic**: Write agents that do one thing well, Orra handles the rest
3. **Stay Flexible**: No forced patterns, no rigid structures, no mandatory "crew" abstractions
4. **Scale Naturally**: Same code works whether your agents are in one file or distributed globally
5. **Ship Confidently**: Production-ready from day one - just split and deploy when ready

## The Orra Advantage

```python
# This seemingly simple setup is incredibly powerful:
researcher = OrraAgent(
    name="research-agent",
    description="Researches topics using web search and knowledge base"
)

writer = OrraAgent(
    name="writing-agent",  
    description="Crafts polished content from research"
)

formatter = OrraService(
    name="format-service",
    description="Formats and validates final content"
)
```

Behind the scenes, Orra's Plan Engine:
- Dynamically figures out how agents should interact
- Handles all messaging and retries
- Manages state and failure recovery
- Scales from local to distributed seamlessly
- Requires zero orchestration code from you

Let's see how this works in practice...

## Quick Example

First, follow the [Echo Example](../examples/echo-python) setup instructions to run Orra locally and create your project. 

Then start prototyping:

```python
from orra import OrraService, OrraAgent, Task
from pydantic import BaseModel, Field
from typing import List
import asyncio

# --- Models ---

class ResearchInput(BaseModel):
    topic: str
    depth: str = Field(default="medium", description="Research depth: basic, medium, deep")

class ResearchOutput(BaseModel):
    summary: str
    key_points: List[str]
    sources: List[str]

class WritingInput(BaseModel):
    content: str
    style: str = Field(default="professional")
    
class WritingOutput(BaseModel):
    text: str
    word_count: int

async def main():
    # --- Researcher Agent ---
    
    researcher = OrraAgent(
       name="research-agent",
       description="Researches topics using web search and knowledge base",
       url="https://api.orra.dev",
       api_key="sk-orra-..." # Orra API key
    )

    @researcher.handler()
    async def research(task: Task[ResearchInput]) -> ResearchOutput:
        """Research handler using your preferred tools (Tavily, web search, etc)"""
        # Your research implementation
        research_result = await your_research_function(task.input.topic, task.input.depth)
        return ResearchOutput(
            summary=research_result.summary,
            key_points=research_result.points,
            sources=research_result.sources
        )

    # --- Writer Agent ---
    
    writer = OrraAgent(
       name="writing-agent",
       description="Crafts polished content from research",
       url="https://api.orra.dev",
       api_key="sk-orra-..." # Same key, Orra handles routing
    )

    @writer.handler()
    async def write(task: Task[WritingInput]) -> WritingOutput:
        """Writing handler using your LLM of choice"""
        # Your writing implementation
        written_content = await your_llm_function(task.input.content, task.input.style)
        return WritingOutput(
            text=written_content,
            word_count=len(written_content.split())
        )

    # --- Formatter Service ---
    
    formatter = OrraService(
       name="format-service",
       description="Formats and validates final content",
       url="https://api.orra.dev",
       api_key="sk-orra-..." # Same key, Orra handles routing
    )

    @formatter.handler()
    async def format(task: Task[WritingInput]) -> WritingOutput:
        """Formatting service - could be stateless language processing"""
        # Your formatting implementation
        formatted = await your_formatter(task.input.text)
        return WritingOutput(
            text=formatted,
            word_count=len(formatted.split())
        )

    # --- Start all services concurrently ---

    # Start service
    await asyncio.gather(
       researcher.start(),
       writer.start(),
       formatter.start()
    )

    try:
       await asyncio.get_running_loop().create_future()
    except KeyboardInterrupt:
       await asyncio.gather(
          researcher.shutdown(),
          writer.shutdown(),
          formatter.shutdown()
       )

if __name__ == "__main__":
   asyncio.run(main())
```

## Usage

1. Run your agents:
```bash
python agents.py
```

2. Orchestrate with CLI:
```bash
# Research and write about AI
orra verify run 'Research and write an article about AI trends' \
  --data topic:'AI in 2024' \
  --data style:'technical'
```

Orra's Plan Engine automatically:
- Determines the optimal execution flow
- Handles all agent and services communication
- Provides detailed logs of agent and services input, outputs 
- Manages retries and failures
- Provides detailed logs of agent and services failures
- Scales execution based on dependencies

## Best Practices

1. **Model Design**
    - Use Pydantic for validation
    - Clear field descriptions
    - Sensible defaults

2. **Error Handling**
    - Let agents handle retries
    - Throw on permanent failures
    - Orra manages recovery

3. **Development Flow**
   ```
   Prototype (single file)
     → Test & Refine
       → Split Services
         → Deploy
   ```

## Going to Production

When ready, split into separate components:
```plaintext
ai-app/
  ├── researcher/
  │   └── main.py  # Just researcher code
  ├── writer/
  │   └── main.py  # Just writer code
  └── formatter/
      └── main.py  # Just formatter code
```

Orra orchestrates them the same way!

## Tips

1. **Development**
    - Use shared utilities
    - Test locally first
    - Monitor execution logs

2. **Debugging**
   ```bash
   # Watch execution
   orra ps
   
   # Inspect flow
   orra inspect -d <orchestration-id>
   
   ```

3. **Advanced Features**
    - Custom retries per agent
    - Stateful agents
    - Parallel execution
    - Webhook notifications

Need help? Check out our [examples](../examples) or join us on [GitHub Discussion](https://github.com/orra-dev/orra/discussions).
