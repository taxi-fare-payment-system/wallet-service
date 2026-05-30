# Audit logs

The analytics service stores platform audit trail entries in the `audit_logs` table. Entries are ingested from RabbitMQ and queried via a paginated REST API.

## Routing key

| Routing key | Exchange | Binding |
|-------------|----------|---------|
| `analytics.audit.recorded` | `analytics_exchange` (config: `ANALYTICS_EXCHANGE`) | Existing consumer queue binding `analytics.#` |

Publishers must send to `analytics_exchange` with routing key **`analytics.audit.recorded`**.

## Event payload (required fields)

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `id` | UUID string | Recommended | Idempotency key; also used as `audit_logs.id`. If omitted, the service generates one. |
| `action` | string | **Yes** | Stable action name, e.g. `user.banned`, `vehicle.approved` |
| `actor_user_id` | string | Recommended | User who performed the action |
| `actor_user_role` | string | Recommended | Role at time of action: `admin`, `superadmin`, `driver`, etc. |
| `target_type` | string | Optional | Entity type, e.g. `user`, `vehicle`, `wallet` |
| `target_id` | string | Optional | Entity identifier |
| `sub_city_id` | uint / numeric string | Optional | Sub-city scope when applicable |
| `metadata` | object | Optional | Extra context (reason, old/new values). Do not include secrets or tokens. |
| `occurred_at` | RFC3339 timestamp | Optional | Defaults to ingest time (UTC) |

### Example publish payload

```json
{
  "id": "a1b2c3d4-e5f6-4789-a012-3456789abcde",
  "action": "user.banned",
  "actor_user_id": "admin-uuid",
  "actor_user_role": "admin",
  "target_type": "user",
  "target_id": "user-uuid",
  "sub_city_id": 101,
  "metadata": {
    "reason": "policy violation"
  },
  "occurred_at": "2026-05-31T10:00:00Z"
}
```

## Idempotency

- The consumer uses the event `id` for deduplication via `analytics_events` (same pattern as other analytics events).
- `audit_logs` inserts use `ON CONFLICT (id) DO NOTHING` so redeliveries do not fail the consumer.

## Publisher checklist

1. Publish to `analytics_exchange` with routing key `analytics.audit.recorded`.
2. Include `id`, `action`, `actor_user_id`, and `actor_user_role` on every message.
3. Set `occurred_at` to the real action time when known.
4. Keep `metadata` small and free of credentials/PII you should not retain.

See also: `docs/interface.md` for auth, RBAC, and shared query conventions.
