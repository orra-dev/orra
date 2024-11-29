# Advanced Topics & Internals

## How Orra Makes Multi-Agent Apps Production-Ready

### Execution Plans

When you submit an action, Orra's AI analyzes your intent and creates an execution plan optimized for reliability and performance:

```json
{
  "tasks": [
    {
      "id": "task0",
      "input": {
        "customerId": "CUST789",
        "orderId": "ORD456"
      }
    },
    {
      "id": "task1",
      "service": "customer-service",
      "input": {
        "customerId": "$task0.customerId"
      }
    },
    {
      "id": "task2",
      "service": "order-system",
      "input": {
        "orderId": "$task0.orderId"
      }
    },
    {
      "id": "task3",
      "service": "ai-agent",
      "input": {
        "customerData": "$customer_context",
        "orderData": "$order_details"
      }
    }
  ],
  "parallel_groups": [
    ["task1", "task2"],
    ["task3"]
  ]
}
```

Key features:
- `task0` holds your original parameters
- Services and agents reference data using `$task.field` syntax
- `parallel_groups` maximizes performance by running independent tasks together

### Exactly-Once Task Execution

Orra uses an idempotency system inspired by Stripe and AWS Lambda to ensure tasks run exactly once, even if services crash or messages are duplicated:

1. Every task gets a unique execution ID
2. Services maintain a 24-hour history of task results
3. Duplicate task requests return cached results
4. Failed tasks don't get retried (your service handles retries)

This means:
- No accidental duplicate charges
- No double-sending of notifications
- No redundant API calls
- Clear task history for debugging

### Service Health & Recovery

Orra actively monitors service health:
- Tracks WebSocket connection status
- Detects unresponsive services
- Maintains service versioning
- Preserves service identity across restarts

When things go wrong:
1. Services can recover and reconnect within 30 minutes
2. In-progress tasks resume automatically
3. No manual intervention needed

### Log-Based Task Coordination

Orra uses an append-only log (similar to Kafka) for task coordination:
- Ordered record of all task events
- Immutable history for debugging
- Basis for exactly-once execution
- Clean recovery after failures

Example log sequence:
```
[0001] Task customer_context started
[0002] Task order_details started
[0003] Task customer_context completed
[0004] Task order_details completed
[0005] Task generate_response started
[0006] Task generate_response completed
```

### Advanced Usage Patterns

#### 1. Dynamic Service/Agent Registration

Services and Agents can update their capabilities at runtime:
```javascript
await agent.register('ai-agent', {
  schema: {
    input: {
      // New capability added
      type: 'object',
      properties: {
        mode: { 
          type: 'string',
          enum: ['fast', 'detailed']
        }
      }
    }
  }
});
```

#### 2. Custom Persistence

Control how service identity persists:

```javascript
const service = initService({
	persistenceOpts: {
		method: 'custom',
		customSave: async (id) => {
			await redis.set(`service:${id}`, id);
		},
		customLoad: async () => {
			return await redis.get('service:id');
		}
	}
});
```

## Production Debugging

Reference [docs/cli.md](cli.md) for inspection commands.

Every orchestration provides detailed inspection:

```shell
orra inspect -d o_xxxxxxxxxxxxxx

┌─ Task Execution Details
│
│ inventory-service (task1)
│ ──────────────────────────────────────────────────
│ 14:07:43  ◎ Processing
│ 14:07:43  ● Completed
│
│ Input:
│   {
│     "productDescription": "A Peanuts collectible Swatch with red straps"
│   }
│
│ Output:
│   {
│     "productId": "697d1744-88dd-4139-beeb-b307dfb1a2f9",
│     "productDescription": "A Peanuts collectible Swatch with red straps",
│     "productAvailability": "AVAILABLE",
│     "warehouseAddress": "Unit 1 Cairnrobin Way, Portlethen, Aberdeen AB12 4NJ"
│   }
│ · · · ·
...
...
```
