# Model Configuration for the Orra Plan Engine

The Orra Plan Engine supports various reasoning and embedding models to power its orchestration capabilities. This document explains the supported models, configuration options, and hosting environments.

## Supported Models

### Reasoning Models

The Plan Engine supports the following reasoning models:

| Model | Description | Recommended Use Cases                                              |
|-------|-------------|--------------------------------------------------------------------|
| `o1-mini` | OpenAI's smaller reasoning model | General purpose orchestration for most applications                |
| `o3-mini` | OpenAI's enhanced reasoning model | Complex orchestration scenarios requiring deeper reasoning         |
| `deepseek-r1` | DeepSeek's reasoning model | Open source alternative with strong reasoning capabilities         |
| `qwq-32b` | Qwen's 32B parameter model | On-prem or self-hosted deployments, privacy-sensitive applications |

### Embedding Models

The Plan Engine supports the following embedding models:

| Model | Description | Recommended Use Cases |
|-------|-------------|----------------------|
| `text-embedding-3-small` | OpenAI's embedding model | General purpose embedding for most applications |
| `jina-embeddings-v2-small-en` | Jina AI's embedding model | Open source alternative for self-hosted deployments |

## Model Variants

The Plan Engine supports model variants with specific suffixes for specialized hardware, optimizations, or context windows:

- Hardware-specific optimization (e.g., `deepseek-r1-mlx`)
- Quantization levels (e.g., `qwq-32b-q8`, `deepseek-r1-q4`)
- Context window sizes (e.g., `model-8k`)
- Version specifications (e.g., `model-v1`, `model-v2.5`)
- Simple alphanumeric suffixes (e.g., `model-beta`, `model-cuda`)

## Configuration

The Plan Engine uses environment variables for model configuration. These can be:
- Set in your environment
- Specified in the `.env` file in the `planengine` directory
- Set in the `_env` template file when deploying

### Basic Configuration

```bash
# Reasoning Model Configuration
LLM_MODEL=o1-mini                           # Choose your preferred reasoning model
LLM_API_KEY=your_api_key                    # Optional if your endpoint doesn't require auth
LLM_API_BASE_URL=https://api.openai.com/v1  # Default for OpenAI, change for self-hosted/other providers

# Embedding Model Configuration
EMBEDDINGS_MODEL=text-embedding-3-small     # Choose your preferred embedding model
EMBEDDINGS_API_KEY=your_api_key             # Optional if your endpoint doesn't require auth
EMBEDDINGS_API_BASE_URL=https://api.openai.com/v1  # Default for OpenAI, change for self-hosted
```

## Hosting Options

Orra allows flexible hosting options for models to meet different operational needs.

### Cloud-based Hosting

#### OpenAI and Azure

**Recommended Models**: 
- `o1-mini` and `o3-mini` via OpenAI or Azure
- `text-embedding-3-small` via OpenAI or Azure

**Configuration Example for OpenAI**:
```bash
LLM_MODEL=o1-mini
LLM_API_KEY=your_openai_key
LLM_API_BASE_URL=https://api.openai.com/v1

EMBEDDINGS_MODEL=text-embedding-3-small
EMBEDDINGS_API_KEY=your_openai_key
EMBEDDINGS_API_BASE_URL=https://api.openai.com/v1
```

**Configuration Example for Azure**:
```bash
LLM_MODEL=o1-mini
LLM_API_KEY=your_azure_key
LLM_API_BASE_URL=https://your-resource.openai.azure.com/openai/deployments/your-deployment/completions?api-version=2023-05-15

EMBEDDINGS_MODEL=text-embedding-3-small
EMBEDDINGS_API_KEY=your_azure_key
EMBEDDINGS_API_BASE_URL=https://your-resource.openai.azure.com/openai/deployments/your-embedding-deployment/embeddings?api-version=2023-05-15
```

#### AI Model Hosting Platforms

AI model hosting platforms provide an excellent option for accessing open source models without managing infrastructure. This is particularly useful for `deepseek-r1`, `qwq-32b`, and `jina-embeddings-v2-small-en`.

**Popular Platforms**:
- [Together AI](https://www.together.ai)
- [Fireworks AI](https://fireworks.ai)
- [HuggingFace Inference API](https://huggingface.co/inference-api)
- [Replicate](https://replicate.com)

**Configuration Example for Together AI**:
```bash
# For DeepSeek-R1
LLM_MODEL=deepseek-r1
LLM_API_KEY=your_together_ai_key
LLM_API_BASE_URL=https://api.together.xyz/v1

# For Jina Embeddings
EMBEDDINGS_MODEL=jina-embeddings-v2-small-en
EMBEDDINGS_API_KEY=your_together_ai_key
EMBEDDINGS_API_BASE_URL=https://api.together.xyz/v1
```

**Configuration Example for Fireworks AI**:
```bash
# For QwQ-32B
LLM_MODEL=qwq-32b
LLM_API_KEY=your_fireworks_key
LLM_API_BASE_URL=https://api.fireworks.ai/inference/v1

# For Jina Embeddings
EMBEDDINGS_MODEL=jina-embeddings-v2-small-en
EMBEDDINGS_API_KEY=your_fireworks_key
EMBEDDINGS_API_BASE_URL=https://api.fireworks.ai/inference/v1
```

**Benefits of Using AI Hosting Platforms**:

1. **Quick Setup**: No need to manage infrastructure or worry about hardware requirements
2. **Cost Efficiency**: Pay-as-you-go pricing models often make these platforms cost-effective
3. **Model Availability**: Easy access to the latest open-source models without deployment hassles
4. **Scalability**: Built-in scaling based on your usage patterns
5. **API Compatibility**: These platforms typically offer OpenAI-compatible APIs, making integration seamless

### On-premises Deployment

**Recommended Models**:
- `qwq-32b` for reasoning
- `jina-embeddings-v2-small-en` for embeddings

**Configuration Example**:
```bash
LLM_MODEL=qwq-32b
LLM_API_KEY=your_internal_key              # If authentication is required
LLM_API_BASE_URL=http://internal-llm-endpoint:8000/v1

EMBEDDINGS_MODEL=jina-embeddings-v2-small-en
EMBEDDINGS_API_KEY=your_internal_key       # If authentication is required
EMBEDDINGS_API_BASE_URL=http://internal-embedding-endpoint:8000/v1
```

### Local Development

**Recommended Models**:
- `qwq-32b` with quantization for reasoning
- `jina-embeddings-v2-small-en` for embeddings

**Configuration Example**:
```bash
LLM_MODEL=qwq-32b-q8
LLM_API_KEY=                               # Often not needed for local deployments
LLM_API_BASE_URL=http://localhost:8000/v1

EMBEDDINGS_MODEL=jina-embeddings-v2-small-en
EMBEDDINGS_API_KEY=                        # Often not needed for local deployments
EMBEDDINGS_API_BASE_URL=http://localhost:8001/v1
```

## Performance Considerations

When choosing your model configuration, consider these factors:

1. **Reasoning Complexity**: For complex multi-agent orchestration with sophisticated reasoning, prefer `o3-mini` or `deepseek-r1`

2. **Latency Requirements**: For lower latency, choose:
   - Cloud providers with regional deployments closer to your application
   - Models with smaller parameter sizes like `o1-mini`
   - Quantized versions of larger models (e.g., `qwq-32b-q8`)

3. **Privacy and Security**: For sensitive workloads requiring data privacy:
   - Use Azure OpenAI with your own dedicated endpoint
   - Deploy open-source models (`deepseek-r1`, `qwq-32b`, `jina-embeddings-v2-small-en`) in your own environment
   - Ensure proper network isolation and access controls

4. **Resource Constraints**: For resource-limited environments:
   - Use quantized models (models with `-q4` or `-q8` suffixes)
   - Consider hardware-optimized versions (e.g., `-mlx` for Apple Silicon)
