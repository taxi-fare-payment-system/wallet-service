# wallet_service — Interface

This document describes the **HTTP interface** exposed by this wallet service: endpoint, request/response shapes, and a short description.

## Conventions

- **Base URL**: `http://<host>:<port>`
- **Content-Type**: JSON unless otherwise noted
- **Request ID**: optional `X-Request-ID` header is accepted; the service echoes/sets `X-Request-ID` on responses.
- **Error response shape** (most non-2xx responses):

```json
{ "message": "..." }
```

- **Wallet object** (returned by wallet endpoints):

```json
{
  "id": 1,
  "user_id": 123,
  "wallet_type": "passenger",
  "freezed": false,
  "balance": "0",
  "created_at": "2026-03-25T12:00:00Z",
  "updated_at": "2026-03-25T12:00:00Z"
}
```

Notes:

- `wallet_type ∈ {"passenger","driver","owner"}`
- `balance` is encoded as a decimal string
- timestamps are RFC3339Nano

---

## Health

### `GET /healthz`

- **Description**: liveness probe
- **Response 200**:

```json
{ "status": "ok" }
```

### `GET /readyz`

- **Description**: readiness probe (includes DB ping)
- **Response 200**:

```json
{ "status": "ok" }
```

- **Response 503** (DB not ready):

```json
{ "status": "not_ready" }
```

---

## Wallets

### `GET /:id`

- **Description**: fetch wallet by wallet id
- **Response 200**: Wallet object
- **Errors**:
  - 400 `{ "message": "invalid wallet id" }`
  - 404 `{ "message": "wallet not found" }`

### `GET /users/:userId?type=<wallet_type>`

- **Description**: fetch a user’s wallet by user id **and wallet type**
- **Query params**:
  - `type` (**required**): `passenger | driver | owner`
- **Response 200**: Wallet object
- **Errors**:
  - 400 `{ "message": "invalid user id" }`
  - 400 `{ "message": "missing wallet type" }`
  - 400 `{ "message": "invalid wallet type" }`
  - 404 `{ "message": "wallet not found" }`

### `POST /`

- **Description**: create a wallet (one per `(user_id, wallet_type)`)
- **Request (JSON)**:

```json
{
  "user_id": 123,
  "type": "passenger"
}
```

- **Response 201**: Wallet object
- **Errors**:
  - 400 `{ "message": "invalid json body" }`
  - 400 `{ "message": "invalid wallet type" }`
  - 400 `{ "message": "invalid user id" }`
  - 409 `{ "message": "wallet already exists for user and type" }`

---

## Top-up

### `PUT /:wallet_id/topup`

- **Description**: create a payment-service checkout session for topping up a **passenger** wallet
- **Request (JSON)**:

```json
{
  "amount": 10,
  "phone_number": "+251900000000",
  "first_name": "First",
  "last_name": "Last",
  "email": "user@example.com",
  "message": "optional note"
}
```

- **Response 200**:

```json
{
  "transaction_id": "<uuid>",
  "checkout_url": "<url>"
}
```

- **Errors**:
  - 400 `{ "message": "invalid wallet id" }`
  - 400 `{ "message": "invalid json body" }`
  - 400 `{ "message": "amount must be > 0" }`
  - 400 `{ "message": "phone_number, first_name, and last_name are required" }`
  - 403 `{ "message": "wallet is frozen" }`
  - 403 `{ "message": "topup is only allowed for passenger wallets" }`
  - 404 `{ "message": "wallet not found" }`
  - 502 `{ "message": "<payment service error>" }`

### `POST /v1/wallet/finalize-topup`

- **Description**: callback endpoint called by payment service when a top-up succeeds; credits the wallet **idempotently**
- **Request (JSON)** (from `payment_service_spec.md`):

```json
{
  "transaction_id": "<uuid>",
  "tx_ref": "pay-<uuid>",
  "chapa_reference": "<reference>",
  "payer_user_id": "<string>",
  "receiver_wallet_id": "<string>",
  "amount": "<decimal string>",
  "currency": "ETB"
}
```

- **Response 200**:

```json
{ "received": true }
```

- **Response 400/500**:

```json
{ "received": false }
```

---

## Pay fare

### `PUT /:wallet_id/pay-fare`

- **Description**: transfer funds from a passenger wallet to a driver wallet (atomic), then records the movement in payment service ledger
- **Request (JSON)**:

```json
{
  "amount": 5,
  "driver_wallet_id": 2,
  "trip_id": "trip-uuid",
  "receiver_full_name": "Driver Name",
  "message": "optional note"
}
```

- **Response 200**:

```json
{
  "success": true,
  "transaction_id": "<uuid>",
  "receipt_url": "<url or null>"
}
```

- **Errors** (non-exhaustive):
  - 400 `{ "message": "invalid wallet id" }`
  - 400 `{ "message": "invalid json body" }`
  - 400 `{ "message": "trip_id is required" }`
  - 400 `{ "message": "receiver_full_name is required" }`
  - 403 `{ "message": "only passenger wallets can pay fare" }`
  - 403 `{ "message": "wallet is frozen" }`
  - 400 `{ "message": "insufficient balance" }`
  - 502 `{ "message": "<trip/payment service error>" }`

Note: pay-fare requires trip validation to be configured (`TRIP_SERVICE_BASE_URL`).

---

## Transactions (proxy)

### `GET /transactions`

- **Description**: proxies payment service `GET /transactions` with restricted query params
- **Allowed query params**:
  - filters: `reason`, `status`, `sender_wallet_id`, `receiver_wallet_id`
  - sorting: `sort`, `order`
  - pagination: `limit` (0–200), `offset` (≥ 0)
- **Forbidden query params**:
  - `payer_user_id`, `trip_id`
- **Response 200**: payment service response shape, e.g.

```json
{
  "items": [],
  "limit": 50,
  "offset": 0,
  "sort": "created_at",
  "order": "desc"
}
```

- **Errors**:
  - 400 `{ "message": "query param not supported: payer_user_id" }`
  - 400 `{ "message": "unknown query param: <name>" }`
  - 400 `{ "message": "invalid limit" }`
  - 400 `{ "message": "invalid offset" }`
  - 502 `{ "message": "<payment service error>" }`

---

## Withdraw

### `PUT /:wallet_id/withdraw`

- **Description**: deducts wallet balance for driver/owner withdrawals (external payout integration is deferred)
- **Request (JSON)**:

```json
{ "amount": 1 }
```

- **Response 202**:

```json
{ "status": "accepted" }
```

- **Errors** (non-exhaustive):
  - 400 `{ "message": "invalid wallet id" }`
  - 400 `{ "message": "invalid json body" }`
  - 403 `{ "message": "wallet is frozen" }`
  - 400 `{ "message": "insufficient balance" }`

---

## Admin: freeze

### `PUT /:wallet_id/freeze`

- **Description**: freezes a wallet (admin-only)
- **Headers**:
  - `X-Admin-User-Id` (**required**): admin user id (int64)
- **Response 200**:

```json
{ "success": true, "wallet_id": 1 }
```

- **Errors** (non-exhaustive):
  - 401 `{ "message": "missing or invalid admin user id" }`
  - 403 `{ "message": "admin access required" }`
  - 503 `{ "message": "auth service not configured" }`
  - 502 `{ "message": "auth service error" }`

---

## Admin: find wallets

### `GET /admin/wallets`

- **Description**: admin-only wallet search with filtering, sorting, and pagination
- **Headers**:
  - `X-Admin-User-Id` (**required**): admin user id (int64)
- **Query params**:
  - **filters**:
    - `user_id` (int64)
    - `wallet_type` (`passenger|driver|owner`)
    - `freezed` (`true|false`)
    - `min_balance` (decimal string)
    - `max_balance` (decimal string)
  - **sorting**:
    - `sort` = `id` (default) | `balance` | `created_at` | `updated_at`
    - `order` = `desc` (default) | `asc`
  - **pagination**:
    - `limit` default 50, max 200
    - `offset` default 0
- **Response 200**:

```json
{
  "items": [ /* wallet objects */ ],
  "limit": 50,
  "offset": 0,
  "sort": "id",
  "order": "desc"
}
```

- **Errors** (non-exhaustive):
  - 401 `{ "message": "missing or invalid admin user id" }`
  - 403 `{ "message": "admin access required" }`
  - 503 `{ "message": "auth service not configured" }`
  - 502 `{ "message": "auth service error" }`
  - 400 `{ "message": "invalid limit" }`
  - 400 `{ "message": "invalid offset" }`
  - 400 `{ "message": "invalid sort" }`
  - 400 `{ "message": "invalid order" }`
  - 400 `{ "message": "invalid user_id" }`
  - 400 `{ "message": "invalid wallet_type" }`
  - 400 `{ "message": "invalid freezed" }`
  - 400 `{ "message": "invalid min_balance" }`
  - 400 `{ "message": "invalid max_balance" }`

---

## Delete wallet

### `DELETE /:wallet_id`

- **Description**: deletes a wallet if its balance is zero
- **Response 204**: no content
- **Errors**:
  - 400 `{ "message": "invalid wallet id" }`
  - 400 `{ "message": "wallet balance must be zero" }`
  - 404 `{ "message": "wallet not found" }`

