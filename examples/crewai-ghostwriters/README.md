# Ghostwriters Example (CrewAI)

Quick [CrewAI](https://www.crewai.com) agentic writer and editor system that drafts stories based on a topic.

## Purpose:

- Showcases how to use Orra with Crewai
- Showcases how to use Orra with Python

## Prerequisites:

- [Docker](https://docs.docker.com/get-docker/) and [Docker Compose](https://docs.docker.com/compose/install/)
- [Poetry](https://python-poetry.org/docs/#installation)
- [OpenAI API key](https://platform.openai.com/api-keys) for Orra's control plane, `writer_crew` and `editor` Agents

## Setup

1. First, setup Orra by following the [installation instructions](../../README.md#installation):

```bash
# Clone Orra
git clone https://github.com/ezodude/orra
cd orra/controlplane

# Set your OpenAI API key
echo "OPENAI_API_KEY=your-key-here" > .env

# Start the control plane
docker compose up
```

2. Setup your Orra project:

```bash
# Install Orra CLI - if using Linux, otherwise download the latest CLI binary for your platform: https://github.com/orra-dev/orra/releases
curl -L https://github.com/ezodude/orra/releases/download/v0.2.1/orra-linux-amd64 -o /usr/local/bin/orra
chmod +x /usr/local/bin/orra

# Create project, add a webhook and API key
orra projects add my-python-project
orra webhooks add http://host.docker.internal:8888/webhook 
orra api-keys gen ghost-key
```

3. Configure the Ghost Writer agents:

```bash
cd examples/crewai-ghost-writers
cp _env .env
```

Edit the file and update the Environment variables accordingly. Use the generated `ghost-key` Orra API Key as the value
for `ORRA_API_KEY`.

## Running the Example

1. Ensure the control plane is running, then start the webhook server (in a separate terminal):

```bash
# Start the webhook server using the verify subcommand
orra verify webhooks start http://localhost:8888/webhook
```

Switch back to the main shell tab to run your agents.

2. Start and register the Ghostwriters' agents:

```bash
# With Poetry
poetry install
poetry run python src/main.py
```

3. Trigger the blog posting generation:

```bash
# Send a test message
orra verify run 'Draft a blog post' \
--data topics_file_path:'/path/to/crewai-ghostwriters/writer-topic/fisherman-story.txt' \
--data output_path:'/path/to/crewai-ghostwriters/drafts/draft.txt' \
-t '5m' \
-w http://host.docker.internal:8888/webhook
```

4. Check the result:

```bash
orra ps
orra inspect -d <orchestration-id>
```

## Notes

This example uses an increased orchestration timeout of 5 minutes (default is 30s).

In this context the timeout was configured using the CLI's `verify` command using the `--timeout` flag (`-t` for short).

## Watch out for CrewAI shenanigans

Sometimes the `writer` Crew get stuck writing and re-writing infinitely. This is a
known [ReACT prompt](https://www.promptingguide.ai/techniques/react) issue, where the prompt repetitively invokes the same function over and over.

Here, Orra is very patient because timeout has been increased to `5m`. But it does kill the orchestration after a while.
However, feel free to kill the Agent running process and start it again WITHOUT stopping the control plane.

Generally, you can keep the control plane running, while you work and update the CrewAI Agent code. Then simply,

- `ctrl+c` to stop the running agents
- Then, run the agents again using `poetry run python src/main.py`.

## Orra with CrewAI 

That's it! Orra provides:

- Service discovery
- Health monitoring
- Reliable task execution
- Error recovery

## Learn More

- [Orra Documentation](../../docs)
- [CLI Documentation](../../docs/cli.md)
