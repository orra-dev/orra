# Orra CLI

The Orra CLI helps you manage your multi-agent apps in development and production.

## Table of Contents

* [Installation](#installation)
* [CLI Commands Reference](#cli-commands-reference)
* [Quick Start](#quick-start)
* [Detailed Command Reference](#detailed-command-reference)
  * [Managing Projects](#managing-projects)
  * [Webhooks Management](#webhooks-management)
  * [API Keys Management](#api-keys-management)
  * [Orchestration Actions](#orchestration-actions)
  * [Reasoning Traces with inspect](#reasoning-traces-with-inspect)
  * [Grounding Management](#grounding-management)
  * [Configuration Management](#configuration-management)
* [Status Icons](#status-icons)
* [Tips for Success](#tips-for-success)
* [Configuration](#configuration)
* [Need Help?](#need-help)

## Installation

Download the latest binary for your platform from our [releases page](https://github.com/orra-dev/orra/releases):

```bash
# macOS/Linux: Move to your PATH
sudo mv ./orra /usr/local/bin/orra
chmod +x /usr/local/bin/orra

# Verify installation
orra version
# Client Version: v0.2.5
```

## CLI Commands Reference

| Command | Description | Example |
| --- | --- | --- |
| `orra projects add` | Add a new project | `orra projects add my-ai-app` |
| `orra projects ls` | List all projects | `orra projects ls` |
| `orra projects use` | Set the current project | `orra projects use my-ai-app` |
| `orra webhooks add` | Add a webhook to the project | `orra webhooks add http://localhost:3000/webhook` |
| `orra webhooks ls` | List all webhooks for a project | `orra webhooks ls` |
| `orra api-keys gen` | Generate an API key for a project | `orra api-keys gen production-key` |
| `orra api-keys ls` | List all API keys for a project | `orra api-keys ls` |
| `orra verify run` | Orchestrate an action with data parameters | `orra verify run "Process order" -d orderId:1234` |
| `orra verify webhooks start` | Start a webhook server for testing | `orra verify webhooks start http://localhost:3000/webhook` |
| `orra ps` | List orchestrated actions for a project | `orra ps` |
| `orra inspect` | Get detailed information about an orchestration | `orra inspect o_abc123` |
| `orra grounding apply` | Apply a grounding spec to a project | `orra grounding apply -f customer-support.yaml` |
| `orra grounding ls` | List all groundings in a project | `orra grounding ls` |
| `orra grounding rm` | Remove grounding from a project | `orra grounding rm customer-support` |
| `orra config reset` | Reset existing Orra configuration | `orra config reset` |
| `orra version` | Print the client and server version | `orra version` |

**CLI flags**:

* `--project`, `-p`: Override the current project for a single command
* `--config`: Specify an alternate config file path

## Quick Start

### 1. Initial Setup

```bash
# Create a new project
orra projects add my-ai-app

# Add a webhook to receive results (assumes the orra Plan Engine is running with docker compose)
orra webhooks add http://host.docker.internal:3000/webhooks/results

# Generate an API key for your services
orra api-keys gen production-key
```

### 2. Register Services

Use the generated API key in your Node.js services:

```javascript
import { initService } from '@orra.dev/sdk';

// Initialize with the API key from step 1
const svc = initService({ 
  name: 'customer-service',
  orraUrl: process.env.ORRA_URL,
  orraKey: 'sk-orra-v1-xyz...' // From orra api-keys gen
});

// Register your service
await svc.register({
  description: 'Handles customer interactions',
  schema: {
    input: {
      type: 'object',
      properties: {
        customerId: { type: 'string' },
        message: { type: 'string' }
      }
    }
  }
});

// Start handling tasks
svc.start(async (task) => {
  // Your service logic here
  
  // Report progress during long-running tasks
  await task.pushUpdate({
    progress: 25,
    message: "Processing customer data..."
  });
  
  // Continue processing and report more progress
  await task.pushUpdate({
    progress: 50,
    message: "Analyzing request..."
  });
  
  // Complete the task with final result
  return { success: true, data: { /* your result */ } };
});
```

### 3. Verify Your Setup

Once your services are running:

```bash
# Test the orchestration
orra verify run "Help customer with delayed order" \
  -d customerId:CUST123 \
  -d orderId:ORD123

# Watch the orchestration progress
orra ps
# ◎ o_abc123  Help customer...  processing  5s ago

# Inspect the details
orra inspect o_abc123
```

Now your services are integrated with Orra and ready for orchestration!

## Detailed Command Reference

### Managing Projects

Projects are containers for orchestration of your multi-agent applications.

```bash
# Create a new project
orra projects add my-ai-app

# List all projects
orra projects ls
# * my-ai-app           Current project
#   customer-service    ID: p_xyz123

# Switch to another project
orra projects use customer-service
```

### Webhooks Management

Webhooks allow Orra to send orchestration results back to your applications.

```bash
# Add a webhook to receive results
orra webhooks add http://localhost:3000/webhook

# List all configured webhooks
orra webhooks ls

# Start a local webhook server for testing
orra verify webhooks start http://localhost:3000/webhook
```

### API Keys Management

API keys are used to authenticate your services with Orra.

```bash
# Generate a new API key
orra api-keys gen staging-key

# List all API keys
orra api-keys ls
# - production-key
#   KEY: sk-orra-v1-abc...
# - staging-key
#   KEY: sk-orra-v1-xyz...
```

### Orchestration Actions

Manage and monitor the running of your multi-agent orchestrations.

```bash
# Submit an action with data
orra verify run "Process refund for order" \
  -d orderId:ORD123 \
  -d amount:99.99

# List all orchestrations
orra ps
# ◎ o_abc123  Process refund    processing  2m ago
# ● o_xyz789  Update inventory  completed   5m ago
# ✕ o_def456  Charge card      failed      8m ago

# Get detailed information about an orchestration
orra inspect o_abc123

# Get comprehensive details with inputs/outputs
orra inspect -d o_abc123

# View progress updates for tasks
orra inspect -d o_abc123 --updates

# View complete progress details for long-running tasks
orra inspect -d o_abc123 --long-updates
```

### Reasoning Traces with inspect

Here's a sample inspection using `orra inspect -d ...`, built to help with reasoning traces.

```shell
$ orra inspect -d o_MdB26xtW9PpbRDbZiqvPTg
Project: assistant
Server:  http://localhost:8005

┌─ Orchestration Details
│ ID:      o_MdB26xtW9PpbRDbZiqvPTg
│ Status:  ● Completed
│ Action:  Can I get it delivered by next week?
│ Created: 141.6h ago
└─────

┌─ Tasks
│ ID       SERVICE                  STATUS         COMPENSATION         DURATION   LAST ERROR
│ ────────────────────────────────────────────────────────────────────────────────────────────────────
│ task1    inventory-service        ● Completed    ─                    101ms      ─
│ task2    delivery-agent.          ● Completed    ─                    11.2s      ─
└─────

┌─ Task Execution Details
│
│ inventory-service (task1)
│ ──────────────────────────────────────────────────
│ 16:09:46  ◎ Processing
│ 16:09:46  ● Completed
│
│ Input:
│   {
│     "action": "checkAvailability",
│     "productId": "laptop-1"
│   }
│
│ Output:
│   {
│     "action": "checkAvailability",
│     "productId": "laptop-1",
│     "status": "product-available",
│     "success": true,
│     "inStock": 1,
│     "message": "Product in stock"
│   }
│
│ · · · · · · · · · · · · · · · · · · · · · · · · ·
│
│ delivery-agent (task2)
│ ──────────────────────────────────────────────────
│ 16:09:46  ◎ Processing
│ 16:09:57  ● Completed
│
│ Input:
│   {
│     "inStock": "$task1.inStock",
│     "productId": "laptop-1",
│     "userId": "user-1"
│   }
│
│ Output:
│   {
│     "status": "delivery-estimated",
│     "success": true,
│     "estimatedDays": 3,
│     "deliveryDate": "2025-03-13",
│     "explanation": "The delivery estimate is calculated based on the current traffic and weather conditions across the route. The best-case scenario assumes optimal speeds with only minor slowdowns, accounting for light traffic on A90, normal conditions with a slight delay on M90 due to roadworks, and the impact of an accident on A1 causing a moderate delay near Darlington. The overall distance is approximately 440 km, and with the high availability of vehicles and light van capacity, a van would be ideal for this delivery. Considering these factors and the current light rain in the Northeast, I anticipate slight slowing but mostly manageable conditions. The worst-case scenario factors in all possible delays reaching their full extent, particularly the 25-minute delay from the accident on A1 and any compounded effects from the widespread light rain, leading to significant, albeit predictable, delays. Given the current conditions and comprehensive data assessment, we can confidently provide these delivery estimates."
│   }
└─────

┌─ Final Results
│
│   {
│     "status": "delivery-estimated",
│     "success": true,
│     "estimatedDays": 3,
│     "deliveryDate": "2025-03-13",
│     "explanation": "The delivery estimate is calculated based on the current traffic and weather conditions across the route. The best-case scenario assumes optimal speeds with only minor slowdowns, accounting for light traffic on A90, normal conditions with a slight delay on M90 due to roadworks, and the impact of an accident on A1 causing a moderate delay near Darlington. The overall distance is approximately 440 km, and with the high availability of vehicles and light van capacity, a van would be ideal for this delivery. Considering these factors and the current light rain in the Northeast, I anticipate slight slowing but mostly manageable conditions. The worst-case scenario factors in all possible delays reaching their full extent, particularly the 25-minute delay from the accident on A1 and any compounded effects from the widespread light rain, leading to significant, albeit predictable, delays. Given the current conditions and comprehensive data assessment, we can confidently provide these delivery estimates."
│   }
└─────
```

### Grounding Management

Grounding helps define domain-specific behaviors for your Orra applications.

```bash
# Apply a grounding spec from a YAML file
orra grounding apply -f customer-support.grounding.yaml

# List all applied groundings
orra grounding ls

# Remove a specific grounding
orra grounding rm customer-support

# Remove all groundings
orra grounding rm --all
```

### Configuration Management

```bash
# Reset the CLI configuration
orra config reset
```

## Status Icons

```
◎ Processing    Task is running
● Completed     Successfully finished
✕ Failed        Error occurred
○ Pending       Waiting to start
⊘ Not Viable    Action cannot be completed
⏸ Paused        Temporarily paused
```

## Tips for Success

1. **Local Development**
```bash
# Start webhook server in one terminal
orra verify webhooks start http://localhost:3000/webhook

# Monitor actions in another
orra ps
```

2. **Debugging Failed Actions**
```bash
# Get detailed execution info
orra inspect -d o_failed123

# Shows full task history, inputs, and outputs
orra inspect -d o_failed123 --updates

# View all progress updates for problematic tasks
orra inspect -d o_failed123 --long-updates
```

3. **Multiple Projects or environments**
```bash
# Switch between projects
orra projects use proj-staging
orra projects use proj-production

# Or use -p flag for one-off commands
orra ps -p proj-production
```

## Configuration

The CLI stores configuration in `~/.orra/config.json`:

```json
{
  "current_project": "my-ai-app",
  "projects": {
    "my-ai-app": {
      "id": "p_abc123",
      "cli_auth": "sk-orra-v1-xyz...",
      "api_keys": {
        "production": "sk-orra-v1-789..."
      },
      "server_addr": "http://localhost:8005"
    }
  }
}
```

Reset if needed:

```bash
orra config reset
```

## Need Help?

1. Check action status:
   ```bash
   orra inspect -d <action-id>
   ```

2. View task progress and updates:
   ```bash
   orra inspect -d <action-id> --updates
   ```

3. View recent actions:
   ```bash
   orra ps
   ```

4. Visit our documentation: [https://orra.dev/docs](https://orra.dev/docs)
