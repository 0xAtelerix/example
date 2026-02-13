# TODO: Failed Bridge Event Recovery Mechanism

## Problem

When a bridge event exhausts all retry attempts (default: 10), it's marked as `Failed` and removed from the retry queue. The user's funds are locked on the source chain with no automated way to recover.

## Proposed Solution

### 1. Admin RPC Endpoint: `retryBridgeEvent`

Add a new JSON-RPC method that allows operators to manually retry a failed bridge event.

**Request:**
```json
{
  "method": "retryBridgeEvent",
  "params": [{ "bridgeId": "0x..." }]
}
```

**Behavior:**
- Validate the bridgeId exists and status is `Failed`
- Reset `RetryCount` to 0, `Status` to `Confirmed`, `ConfirmedAt` to now
- Re-insert into `PendingEventsBucket` so the automatic retry loop picks it up
- Return success/failure

**Files to modify:**
- `application/api/api.go` — add `retryBridgeEvent` RPC method
- `application/api/types.go` — add request/response types
- `application/state.go` — add `ResetBridgeEvent()` helper that updates both buckets

### 2. Admin RPC Endpoint: `retryAllFailedEvents`

Bulk retry all failed events at once.

**Request:**
```json
{
  "method": "retryAllFailedEvents",
  "params": []
}
```

**Behavior:**
- Scan `BridgeEventsBucket` for all events with `Status == "Failed"`
- Reset each one (same as single retry above)
- Return count of events reset

**Files to modify:** Same as above.

### 3. Auth Guard for Admin Endpoints

Admin RPC methods should not be publicly accessible.

**Options (pick one):**
- **API key header** — simplest; check `X-Admin-Key` header against a config value
- **Separate admin port** — bind admin RPCs to a different port only accessible internally
- **IP allowlist** — restrict admin endpoints to localhost / internal IPs

**Recommendation:** Separate admin port (add `admin_port` to `config.yaml`). This keeps the public RPC clean and lets infra handle access control via firewall rules.

### 4. Frontend "Retry" Button (Optional)

Add a retry button on failed transactions in the history tab that calls the admin endpoint.

**Consideration:** This should only be visible to operators/admins, not end users. Either:
- Gate behind an admin mode toggle in the UI
- Or keep it CLI/RPC-only and skip the UI button

### 5. Alerting for Failed Events

Already covered by the existing `BridgeTransactionStuck` alert rule (fires when pending > 5 min). Additionally, consider adding a dedicated alert:

```yaml
- alert: BridgeEventFailed
  expr: increase(bridge_transactions_total{status="retry"}[10m]) > 0 and bridge_transactions_total{status="retry"} >= 10
  for: 0m
  labels:
    severity: critical
  annotations:
    summary: "Bridge event exhausted retries"
    description: "A bridge event has hit max retries and been marked as Failed."
```

**Better approach:** Add a new Prometheus counter `bridge_transactions_failed_total` that increments when an event transitions to Failed status. Then alert on any increment:

```yaml
- alert: BridgeEventFailed
  expr: increase(bridge_transactions_failed_total[5m]) > 0
  for: 0m
  labels:
    severity: critical
  annotations:
    summary: "Bridge event failed"
    description: "{{ $value }} bridge event(s) exhausted retries in the last 5 minutes."
```

**Files to modify:**
- `application/metrics/metrics.go` — add `BridgeTransactionsFailed` counter
- `application/external_block_processor.go` — increment on `BridgeStatusFailed` transition
- `monitoring/alert_rules.yml` — add alert rule

## Implementation Order

1. Admin RPC endpoints (`retryBridgeEvent` + `retryAllFailedEvents`)
2. Auth guard (separate admin port)
3. `bridge_transactions_failed_total` metric + alert rule
4. Frontend retry button (optional, can defer)
