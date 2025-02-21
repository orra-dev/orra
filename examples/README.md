# Orra Examples

Production-ready examples showing how to build reliable multi-agent applications with Orra.

We're starting with JavaScript and more languages are coming soon.

## Available Examples

### [Echo Service (JavaScript)](echo-js)
The "Hello World" of Orra for JS developers - perfect for learning the basics of service orchestration.

### [Echo Service (Python)](echo-python)
The "Hello World" of Orra for Pythonistas - perfect for learning the basics of service orchestration.

### [AI E-commerce Assistant](ecommerce-agent-app)
Complete e-commerce example with multiple services, AI agent integration, and real-time updates.

### [Ghostwriters (Python)](crewai-ghostwriters)
Content generation example showcasing how to use Orra with [CrewAI](https://www.crewai.com). 🆕🎉

## Tips From Production

Having deployed agent architectures in production, here's what to watch for:

- Start small (Echo example) before tackling complex workflows
- Always implement proper error handling and graceful shutdowns
- Monitor your service health and orchestration status using `orra inspect`
- Keep your Mistral/OpenAI API keys secure and never commit them

## Quick Links

- [Documentation](../docs) - Everything you need to know
- [Control Plane Setup](../README.md#2-get-orra-running) - Required before running examples
- [CLI Reference](../docs/cli.md) - Essential commands for development

Need help? Join us on [GitHub Discussion](https://github.com/orra-dev/orra/discussions).
