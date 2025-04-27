# Monitoring with Webhooks

## Overview

Webhooks provide a simple yet powerful mechanism for monitoring your orra orchestrations in real-time. By configuring webhooks, you can receive notifications for key events in your orchestration lifecycle, enabling you to integrate with monitoring systems, logging platforms, or trigger custom business processes in response to orchestration state changes.

orra offers webhooks for three critical events:
1. **Orchestration Completions** - When an orchestration successfully completes
2. **Orchestration Failures** - When an orchestration fails during execution
3. **Orchestration Compensation Failures** - When a compensation operation fails during rollback

This document provides guidance on how to set up, configure, and utilize these webhooks for effective monitoring of your agent applications.

## Setting Up Webhooks

### Prerequisites

- A configured orra project
- A publicly accessible HTTP endpoint to receive webhook notifications

### Configuring Standard Webhooks

Standard webhooks receive notifications for orchestration completions and failures. Use the CLI to add a webhook to your project:

```bash
# Add a webhook for orchestration completions and failures
orra webhooks add https://your-webhook-endpoint.com/orra-notifications
```

Add as many webhooks as you like for your project.

### Configuring Compensation Failure Webhooks

Compensation failure webhooks are managed separately to allow for dedicated handling of these critical events:

```bash
# Add a webhook specifically for compensation failures
orra comp-fail webhooks add https://your-webhook-endpoint.com/compensation-failures
```

### Listing Configured Webhooks

You can view your configured webhooks using the CLI:

```bash
# List standard webhooks
orra webhooks ls

# List compensation failure webhooks
orra comp-fail webhooks ls
```

## Webhook Event Types and Payloads

### Orchestration Completion Events

When an orchestration successfully completes, your webhook endpoint will receive a POST request with a JSON payload containing:

```json
{
  "event_id": "o_123456-completed-1696415762",
  "type": "orchestration.completed",
  "project_id": "your-project-id",
  "orchestration_id": "o_123456",
  "status": "completed",
  "timestamp": "2023-10-27T15:30:45Z",
  "results": [
    {
      "key": "value"
    }
  ]
}
```

### Orchestration Failure Events

When an orchestration fails, your webhook endpoint will receive a POST request with a JSON payload containing:

```json
{
  "event_id": "o_123456-failed-1696415762",
  "type": "orchestration.failed",
  "project_id": "your-project-id",
  "orchestration_id": "o_123456",
  "status": "failed",
  "timestamp": "2023-10-27T15:45:12Z",
  "error": {
    "id": "<id>",
    "producer": "task1",
	"error": "Task execution failed: service-name failed to process request"
  }
}
```

### Orchestration Compensation Failure Events

When a compensation operation fails, your webhook endpoint will receive a POST request with a detailed JSON payload:

```json
{
  "event_id": "c_123456-failed-1696415762",
  "type": "compensation.failed",
  "compensation_id": "c_123456",
  "project_id": "your-project-id",
  "orchestration_id": "o_123456",
  "task_id": "task1",
  "service_id": "s_123234",
  "service_name": "Payment Service",
  "status": "failed",
  "failure": "Service unavailable: failed to process refund request",
  "timestamp": "2023-10-27T16:10:30Z",
  "compensation_data": {
    "input": {
      "transaction_id": "txn-12345"
    },
    "context": {
      "reason": "orchestration_failed",
      "timestamp": "2023-10-27T16:05:20Z",
      "payload": {
        "error": "Shipping estimation failed"
      }
    }
  }
}
```

## Webhook Headers

All webhook requests include the following HTTP headers:

| Header | Description |
|--------|-------------|
| `Content-Type` | Always `application/json` |
| `User-Agent` | `orra/1.0` |
| `X-Orra-Event` | Event type: `orchestration.completed`, `orchestration.failed`, or `compensation.failed` |

## Managing Compensation Failures

### Human Intervention for Failed Compensations

Compensation failures indicate potential data inconsistencies that often require human intervention. When a compensation failure webhook is received:

1. **Investigate the failure** using the orra CLI to understand what went wrong
2. **Take manual corrective action** if necessary (e.g., manually process a refund)
3. **Record the resolution** using the CLI commands

Each compensation failure will remain in a `pending` state until explicitly marked as `resolved` or `ignored` by an operator.

### CLI Commands for Compensation Management

```bash
# List failed compensations (defaults to showing only pending items)
orra comp-fail ls

# Show all compensations including resolved and ignored
orra comp-fail ls --all

# Inspect detailed information about a specific failure
orra comp-fail inspect <compensation-id>

# Mark as resolved after taking manual corrective action
orra comp-fail resolve <compensation-id> --reason "Manually refunded customer"

# Mark as ignored for cases that don't require action
orra comp-fail ignore <compensation-id> --reason "Test transaction, no actual impact"
```

## Best Practices

### Webhook Endpoint Security

- Use HTTPS for all webhook endpoints
- Consider implementing authentication for webhook requests
- Validate the source of webhook requests in your handler

### Reliable Webhook Processing

- Respond quickly to webhook requests (under 10 seconds)
- Process webhooks asynchronously in your application
- Implement idempotent handling using the `event_id` field to prevent duplicate processing

### Monitoring and Alerting

- Set up real-time alerts for orchestration failures
- Create dedicated alerts for compensation failures
- Establish escalation paths for unresolved compensation failures

## Example Webhook Handler

Here's a simple example of a webhook handler in Node.js:

```javascript
const express = require('express');
const app = express();
app.use(express.json());

// Store processed event IDs to prevent duplicate processing
const processedEvents = new Set();

app.post('/orra-notifications', async (req, res) => {
  const eventType = req.header('X-Orra-Event');
  const payload = req.body;
  const eventId = payload.event_id;
  
  // Check for duplicate events
  if (processedEvents.has(eventId)) {
    console.log(`Duplicate event received: ${eventId}`);
    return res.status(200).send('Event already processed');
  }
  
  console.log(`Received ${eventType} webhook:`, payload);
  
  // Process event asynchronously
  processWebhook(eventType, payload).catch(err => {
    console.error(`Error processing webhook ${eventId}:`, err);
  });
  
  // Store the event ID to prevent reprocessing
  processedEvents.add(eventId);
  
  // Respond quickly to acknowledge receipt
  res.status(200).send('Webhook received');
});

async function processWebhook(eventType, payload) {
  switch (eventType) {
    case 'orchestration.completed':
      await handleOrchestrationComplete(payload);
      break;
    case 'orchestration.failed':
      await handleOrchestrationFailure(payload);
      break;
    case 'compensation.failed':
      await handleCompensationFailure(payload);
      break;
  }
}

async function handleOrchestrationComplete(payload) {
  // Record successful orchestration
  // Update dashboards, etc.
}

async function handleOrchestrationFailure(payload) {
  // Trigger alerts
  // Log details for investigation
}

async function handleCompensationFailure(payload) {
  // High-priority alert
  // Create incident ticket
}

// Prune old event IDs periodically to prevent memory leaks
setInterval(() => {
  const now = Date.now();
  const oneHourAgo = now - (60 * 60 * 1000);
  
  // Remove events older than 1 hour
  // In production, you'd use a more robust solution like Redis with TTL
}, 15 * 60 * 1000); // Run every 15 minutes

app.listen(3000, () => {
  console.log('Webhook server running on port 3000');
});
```

## Integration with Monitoring Systems

### Prometheus Integration

Here's an example of how to expose metrics from your webhook handler for Prometheus:

```javascript
const client = require('prom-client');

// Create metrics
const orchestrationCompletions = new client.Counter({
  name: 'orra_orchestration_completions_total',
  help: 'Total number of successful orchestration completions'
});

const orchestrationFailures = new client.Counter({
  name: 'orra_orchestration_failures_total',
  help: 'Total number of orchestration failures'
});

const compensationFailures = new client.Counter({
  name: 'orra_compensation_failures_total',
  help: 'Total number of compensation failures'
});

// ...

async function handleOrchestrationComplete(payload) {
  // Increment counter
  orchestrationCompletions.inc();
  // Other processing
}

async function handleOrchestrationFailure(payload) {
  // Increment counter
  orchestrationFailures.inc();
  // Other processing
}

async function handleCompensationFailure(payload) {
  // Increment counter
  compensationFailures.inc();
  // Other processing
}

// Expose metrics endpoint
app.get('/metrics', async (req, res) => {
  res.set('Content-Type', client.register.contentType);
  res.end(await client.register.metrics());
});
```

## Conclusion

Webhooks provide a powerful mechanism for monitoring your orra orchestrations in real-time. By implementing proper webhook handling and integrating with your monitoring systems, you can ensure your agent applications are reliable, maintainable, and that any issues are promptly addressed.

**Remember that compensation failures represent potential data inconsistencies in your system and should be prioritized appropriately in your monitoring and response workflows.**
