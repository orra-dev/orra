# Domain Grounding

Domain grounding helps ground Orra's planning system in a project's domain. A grounding spec provides real-world use cases of actions and their execution patterns, helping Orra generate more accurate and reliable execution plans.

## Quick Start

1. Create a domain grounding file:

```yaml
# customer-support.grounding.yaml
name: "customer-support-use-cases"
domain: "e-commerce-customer-support"
version: "1.0"

use-cases:
  - action: "Handle customer inquiry about {orderId}"
    params:
      orderId: "ORD123"
      query: "Where is my order?"
    capabilities:
      - "Lookup order details"
      - "Generate response"
    intent: "Customer wants order status and tracking"

  - action: "Process refund for {orderId}"
    params:
      orderId: "ORD456"
      reason: "Item damaged"
    capabilities:
      - "Verify refund eligibility"
      - "Process payment refund"
    intent: "Handle customer refund request"

constraints:
  - "Verify customer owns order"
  - "Never share other customer data"
```

2. Add the examples to your project using the CLI:

```bash
# Add domain examples
orra grounding apply grounding.yaml

# Test execution planning
orra grounding test examples.yaml
```

## Understanding Domain Grounding

Domain grounding consists of a spec that defines:

1. **Metadata**
   - `name`: Identifier for this set of examples
   - `domain`: The domain these examples belong to
   - `version`: Version of the examples

2. **Use Cases**
   - `action`: Pattern of the action with optional variables in {braces}
   - `params`: Example parameters that would be provided
   - `capabilities`: The required capabilities to handle the action
   - `intent`: Clear description of what the action aims to achieve

3. **Constraints** - OPTIONAL
   - Business rules and limitations that apply to all actions

## Organizing Domain Examples

Organize examples by domain functionality:

```
ecommerce/
   customer-support.grounding.yaml     # Support-related examples
   order-processing.grounding.yaml     # Order-related examples
   inventory.grounding.yaml            # Inventory-related examples
```

Each file should focus on a specific aspect of your domain. This makes examples:
- Easier to maintain
- Clearer to understand
- Simpler to test
- More manageable for teams

## Best Practices

1. **Keep Grounding Simple**
   - Focus on common patterns
   - Use clear, descriptive intents
   - Include essential capabilities only

2. **Use Clear Action Patterns**
   ```yaml
   # Good
   action: "Process refund for {orderId}"
   ```
   
   ```yaml
   # Good
   action: "Process refund for order"
   ```
   
   ```yaml
   # Bad - too vague
   action: "Handle order issue"
   ```

3. **Be Specific with Capabilities**
   ```yaml
   # Good
   capabilities:
     - "Verify refund eligibility"
     - "Process payment refund"
   ```
   
   ```yaml
   # Bad - too generic
   capabilities:
     - "Handle refunds"
   ```

4. **Write Clear Intents**
   ```yaml
   # Good
   intent: "Customer wants order status and tracking"
   ```
   
   ```yaml
   # Bad - too vague
   intent: "Handle order query"
   ```

## Example Use Cases

Here are some spec for common domains:

### Customer Support

```yaml
use-cases:
  - action: "Find order status for {customerId}"
    params:
      customerId: "CUST123"
    capabilities:
      - "Customer verification"
      - "Order history lookup"
    intent: "Look up all orders for customer"

  - action: "Update shipping address for {orderId}"
    params:
      orderId: "ORD456"
      address: "123 New St"
    capabilities:
      - "Order verification"
      - "Address validation"
      - "Shipping update"
    intent: "Change delivery address for order"
```

### Order Processing

```yaml
use-cases:
  - action: "Process payment for {orderId}"
    params:
      orderId: "ORD789"
      amount: "99.99"
    capabilities:
      - "Payment processing"
      - "Order update"
    intent: "Process order payment and update status"

  - action: "Check stock for {productId} in {location}"
    params:
      productId: "PROD123"
      location: "NYC Store"
    capabilities:
      - "Inventory lookup"
      - "Stock verification"
    intent: "Check product availability in specific location"
```

## FAQs

**Q: How many use cases should I include?**  
A: Start with 2-3 key use cases per major action pattern. Add more as needed based on testing results.

**Q: Can I have multiple grounding files?**  
A: Yes! Split use cases by functionality to keep files focused and maintainable.

**Q: How detailed should capabilities be?**  
A: List specific, atomic capabilities. They should match your actual service capabilities.

## Next Steps

- [Orchestrating Actions with Orra](actions.md)
- [Advanced Topics & Internals](advanced.md)
