# Core Topics & Internals

## How Orra Makes Agent Apps Production-Ready

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
- Services, Tasks as Services and Agents reference data using `$task.field` syntax

### Grounding and Execution Plan Validation

Orra's grounding system enforces strict production safety through comprehensive validation of execution plans. This isn't just type checking - it's a complete semantic validation of your application's runtime behavior:

```yaml
name: e-commerce-support
domain: customer-service
version: "1.0"
use-cases:
  - action: "Process refund for order {orderId}"
    capabilities:
      - "Verify refund eligibility"
      - "Process payment refund"
    intent: "Handle customer refund requests safely"
constraints:
  - "Must verify eligibility before processing"
  - "Only one refund per order"
```

Key features:
- Domain actions matched in latent space using cosine similarity
- Language-agnostic capability matching through vector embeddings
- Composition safety through complete capability validation
- Zero-tolerance for capability mismatches in production

Validation operates in two distinct modes:
1. Without grounding:
    - Basic syntax and schema validation
    - No semantic safety guarantees
    - Suitable for initial development
2. With grounding (production mode):
    - Full semantic validation in latent space
    - Strict capability matching
    - Complete task composition verification
    - No partial validation - it's all or nothing

Example validation flow:
```
[validate] Domain expert grounding loaded
[validate] Embedding action "Issue a refund for ORD123"
[validate] Cosine similarity match (0.92) to "Process refund"
[validate] Validating service capabilities:
  ✓ Refund eligibility verification (0.89 similarity)
  ✓ Payment processing capability (0.95 similarity)
[validate] Composition safety verified:
  ✓ All required capabilities present
  ✓ No unsafe task combinations
  ✓ Task ordering preserves constraints
[validate] Plan approved for execution
```

This ensures:
- Zero drift between intended and actual behavior
- No undefined states or partial failures
- Complete validation of task compositions
- Full preservation of domain safety rules
- Clear, binary validation outcomes

When grounding is applied, there's no middle ground - your execution plan either fully satisfies all safety requirements or it doesn't run. This hard boundary prevents the subtle failures that often plague distributed systems where partially incorrect behavior can be worse than complete failure.

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
- Clear task audit history for debugging

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

### Compensations & Recovery

Orra's compensation system provides sophisticated failure recovery for services and agents:
- Tracks completed tasks from revertible services
- Executes compensations in reverse chronological order
- Handles partial and complete compensation results
- Retries failed compensations with exponential backoff (up to 10 attempts)
- Maintains strict TTL on compensation data (default 24h)

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
