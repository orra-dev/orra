# Compensations in orra

## Overview

When building AI agent applications for production, ensuring reliable execution isn't just about retrying failed operations - it's about maintaining system consistency when things go wrong. orra's compensation system provides a sophisticated way to "undo" or "revert" completed operations when a downstream failure occurs that can't be recovered from.

Think of compensations like rolling back a distributed transaction, but for AI agent workflows. If your agent app processes an order but fails during delivery estimation, you need to release any held inventory and notify relevant systems.

## Key Concepts

### Revertible Services & Agents

Services and agents can be marked as "revertible" during registration. This indicates they maintain enough state to undo their operations if needed:

```javascript
// JavaScript Example
await service.register({
   description: "Inventory management ...",
   revertible: true,  // Enable compensations
   schema: {...}
});

// Add compensation handler with context awareness
service.onRevert(async (task, result, context) => {
   console.log('Reverting task:', task.id);
   console.log('Original result:', result);
   console.log('Compensation reason:', context.reason);
   
   await releaseInventoryHold(result.productId);
});
```

```python
# Python Example
service = OrraService(
   name="inventory-service",
   revertible=True,  # Enable compensations
   #...
)

@service.revert_handler()
async def revert_inventory(source: RevertSource[Input, Output]) -> CompensationResult:
   print(f"Reverting inventory hold for: {source.output.product_id}")
   print(f"Compensation reason: {source.context.reason}")
   
   await release_hold(source.output.product_id)
   return CompensationResult(status=CompensationStatus.COMPLETED)
```

### Compensation Context

Compensation handlers now receive context information explaining why the compensation was triggered:

For detailed examples of using compensation context, see:
- JavaScript: [JS SDK Documentation](sdks/js-sdk.md#compensation-context)
- Python: [Python SDK Documentation](sdks/python-sdk.md#compensation-context)

### Compensation Results

Compensations can have four outcomes:

1. **Completed** - The operation was fully reverted
2. **Partial** - Only some aspects could be reverted
3. **Failed** - The compensation itself failed
4. **Expired** - The compensation TTL elapsed before execution could occur

### Configuration

1. **TTL configuration**
   - Set appropriate TTL during service/agent registration
   - Consider business requirements (e.g., refund windows)
   ```javascript
   // JavaScript
   await service.register({
     revertible: true,
     revertTTL: 72 * 60 * 60 * 1000,  // 72 hours in ms
     //...
   });
   ```

   ```python
   # Python
   service = OrraService(
     revertible=True,
     revert_ttl_ms=72 * 60 * 60 * 1000,  # 72 hours in ms
     #...
   )
   ```

2. **Partial compensations**
   - Handle cases where only some operations can be reverted
   - Clearly document what was/wasn't compensated
   ```javascript
   // JavaScript
   return {
     status: 'partial',
     partial: {
       completed: ['inventory_hold'],
       remaining: ['notification']  
     }
   };
   ```

   ```python
   # Python
   return CompensationResult(
       status=CompensationStatus.PARTIAL,
       partial=PartialCompensation(
           completed=['inventory_hold'],
           remaining=['notification']
       )
   )
   ```

3. **Handling failures**
   - If a compensation fails, throw or raise the failure as an error from the revert handler
   - The orra Plan Engine will capture the error and deal with it accordingly

4. **Monitoring and Webhooks**
   - Track compensation attempts and failures through webhooks
   - Set up alerts for failed compensations
   - Handle failed compensations that require manual intervention

   See [Monitoring with Webhooks](monitoring-with-webhooks.md) for complete details on compensation failure monitoring and management.

### How It Works

1. **Storage**: When a revertible service/agent completes a task, the result is stored in the orchestration log for potential compensation.

2. **Background Execution**: When an action fails or is aborted, orra automatically:
   - Identifies all completed revertible tasks
   - Orders them for compensation (newest to oldest)
   - Executes compensations in sequence
   - Provides context about the failure to each compensation handler

3. **Reliability**: Each compensation:
   - Is retried up to 10 times with exponential backoff
   - Has a configurable TTL set in milliseconds (default 24h)
   - Maintains exactly-once execution guarantees

## Example Scenario

Consider an e-commerce flow:

1. Customer service validates order
2. Inventory service holds product (revertible)
3. Payment service processes charge (revertible)
4. Delivery service estimates shipping

If delivery estimation fails permanently, orra will:
1. Revert payment service (refund charge)
2. Revert inventory service (release hold)

Both compensation handlers receive context explaining the failure reason.

## Limitations & Considerations

- Compensations must complete within 30 seconds per attempt
- Maximum 10 retry attempts per compensation
- Compensation data expires after TTL (default 24h)
- Services must be healthy to process compensations
- Failed compensations require human intervention (see [Monitoring with Webhooks](monitoring-with-webhooks.md#human-intervention-for-failed-compensations))

## Conclusion

Compensations are a powerful tool for maintaining system consistency in AI agent applications. By implementing context-aware compensation handlers and monitoring compensation failures, you can ensure your production systems handle failures gracefully and maintain data consistency even in complex workflows.
