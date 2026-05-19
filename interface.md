# wallet_service — Interface

This document describes the **HTTP interface** exposed by this wallet service: endpoint, request/response shapes, and a short description.

## Route index

All paths are under the service base URL and use prefix **`/api/v1/wallet`**.

| Method | Path |
| --- | --- |
| `GET` | `/healthz` |
| `GET` | `/readyz` |
| `GET` | `/banks/chapa` |
| `GET` | `/assistant/:assistantId/earnings` |
| `GET` | `/transactions` |
| `POST` | `/` (base path = create wallet) |
| `GET` | `/users/:userId` |
| `GET` | `/:wallet_id` |
| `PUT` | `/:wallet_id/topup` |
| `POST` | `/finalize-topup` |
| `PUT` | `/:wallet_id/pay-fare` |
| `POST` | `/:wallet_id/transfer` |
| `PUT` | `/:wallet_id/withdraw` |
| `GET` | `/:wallet_id/withdrawals` |
| `PUT` | `/:wallet_id/freeze` |
| `DELETE` | `/:wallet_id` |
| `GET` | `/admin/wallets` |
| `GET` | `/admin/configs` |
| `PUT` | `/admin/configs` |

Sections below follow this order: [Health](#health) → [Banks](#banks-payment-pass-through) → [Wallets](#wallets) → [Top-up](#top-up) → [Pay fare](#pay-fare) → [Wallet transfer (P2P)](#wallet-transfer-p2p) → [Assistant earnings](#assistant-earnings) → [Transactions](#transactions-proxy) → [Withdraw](#withdraw) → [List withdrawals](#list-withdrawals) → [Admin: freeze](#admin-freeze) & [Admin: find wallets](#admin-find-wallets) & [Admin: configs](#admin-configs) → [Delete wallet](#delete-wallet).

## Conventions

- **Base URL**: `http://<host>:<port>` (default listen port **8088**; register with the gateway team.)
- **Content-Type**: JSON unless otherwise noted
- **Request ID**: optional `X-Request-ID` header is accepted; the service echoes/sets `X-Request-ID` on responses.
- **User IDs**: `user_id` values in this service are **strings**. Gateway-injected `X-User-ID` is treated as the same string id.
- **Gateway trust headers** (JWT-validated routes): `X-User-ID`, `X-User-Role`, and for scoped admins `X-Sub-City`.
- **Payment / Trip / Auth calls**: when the wallet service calls Payment, Trip, or Auth on behalf of the user, it forwards the caller’s `Authorization: Bearer …` from the incoming request (e.g. top-up profile lookup, P2P receiver lookup by phone).
- **Error response shape** (most non-2xx responses):

```json
{ "message": "..." }
```

- **Wallet object** (returned by wallet endpoints):

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "user_id": "123",
  "wallet_type": "passenger",
  "freezed": false,
  "balance": "0",
  "created_at": "2026-03-25T12:00:00Z",
  "updated_at": "2026-03-25T12:00:00Z"
}
```

Notes:

- `id` (wallet id) is a **UUID** string
- `wallet_type ∈ {"passenger","driver","owner"}`
- `balance` is encoded as a decimal string
- timestamps are RFC3339Nano

### Integrations (out of band)

- **Analytics** (RabbitMQ): exchange `analytics_exchange` (topic). Events: `analytics.wallet.created`, `analytics.wallet.balance_updated` (topup, fare debit/credit, withdrawal). Env: `RABBITMQ_URL`, `ANALYTICS_EXCHANGE`.
- **Notifications** (RabbitMQ): exchange `notification.exchange` (topic). Events include `notification.wallet.topup_succeeded`, `notification.wallet.pay_fare_succeeded`, `notification.wallet.frozen`, `notification.wallet.withdrawal_initiated`, `notification.wallet.transfer_sent`, `notification.wallet.transfer_received`. Env: `NOTIFICATION_EXCHANGE`.

---

## Health

### `GET /api/v1/wallet/healthz`

- **Description**: liveness probe
- **Response 200**:

```json
{ "status": "ok" }
```

### `GET /api/v1/wallet/readyz`

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

## Banks (payment pass-through)

### `GET /api/v1/wallet/banks/chapa`

- **Description**: forwards `GET` to Payment Service `GET /api/v1/payments/banks/chapa` (Chapa bank list; Payment caches ~24h). Callers use returned `code` values for withdrawals.
- **Auth**: authenticated user; **`Authorization` required** so Payment can authorize the call.
- **Response 200**: Payment Service body, e.g.

```json
{
  "items": [
    {
      "id": "1",
      "name": "Commercial Bank of Ethiopia",
      "slug": "commercial-bank-of-ethiopia",
      "code": "656",
      "currency": "ETB"
    }
  ]
}
```

- **Errors**: `502` / `5xx` if Payment Service fails or is unreachable.

---

## Wallets

### `GET /api/v1/wallet/:wallet_id`

- **Description**: fetch wallet by wallet id
- **Response 200**: Wallet object
- **Errors**:
  - 400 `{ "message": "invalid wallet id" }`
  - 404 `{ "message": "wallet not found" }`

### `GET /api/v1/wallet/users/:userId?type=<wallet_type>`

- **Description**: fetch a user’s wallet by **string** user id (`:userId` path segment) and wallet type. Access-controlled:
  - **Own wallet** (`X-User-ID` equals `:userId`): full Wallet object.
  - **`admin` / `superadmin`**: full Wallet object.
  - **`passenger`** requesting `type=driver`: **only** `{ "wallet_id": <id> }` (no balance or freeze fields), for QR / pay-fare flows.
  - Any other cross-user access: **403**.
- **Headers**: `X-User-ID` and `X-User-Role` are expected from the gateway for authenticated routes.
- **Query params**:
  - `type` (**required**): `passenger | driver | owner`
- **Response 200**: Wallet object, or `{ "wallet_id": "<wallet-uuid>" }` for the passenger→driver case above.
- **Errors**:
  - 400 `{ "message": "invalid user id" }`
  - 400 `{ "message": "missing wallet type" }`
  - 400 `{ "message": "invalid wallet type" }`
  - 403 `{ "message": "forbidden" }`
  - 404 `{ "message": "wallet not found" }`

### `POST /api/v1/wallet`

- **Description**: create a wallet (one per `(user_id, wallet_type)`). Intended for internal calls (e.g. Auth Service after registration). **`user_id` is a string.**
- **Request (JSON)**:

```json
{
  "user_id": "123",
  "type": "passenger"
}
```

- **Response 201**: Wallet object
- **Errors**:
  - 400 `{ "message": "invalid json body" }`
  - 400 `{ "message": "invalid wallet type" }`
  - 400 `{ "message": "invalid user id" }`
  - 409 `{ "message": "wallet already exists for user and type" }` (callers such as Auth may treat this as success on retry)

---

## Top-up

### `PUT /api/v1/wallet/:wallet_id/topup`

- **Description**: create a payment-service checkout session for topping up a **passenger** wallet
- **Request (JSON)**:

```json
{
  "amount": 10,
  "phone_number": "+251900000000",
  "email": "user@example.com",
  "message": "optional note"
}
```

- **Name source**: `first_name` and `last_name` are fetched from Auth Service `GET /api/v1/auth/me` using forwarded `Authorization` bearer token (`display_name` split into first/last parts).
- **Phone source**: `phone_number` is optional in the request; if omitted, Wallet uses `phone` from Auth `/api/v1/auth/me`.

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
  - 401 `{ "message": "authentication error" }` (missing/invalid token or Auth profile lookup failure)
  - 403 `{ "message": "wallet is frozen" }`
  - 403 `{ "message": "topup is only allowed for passenger wallets" }`
  - 404 `{ "message": "wallet not found" }`
  - 502 `{ "message": "<payment service error>" }`

### `POST /api/v1/wallet/finalize-topup`

- **Description**: callback from Payment Service when a top-up succeeds; credits the wallet **idempotently**. Publishes analytics and (on first credit) a notification event.
- **Request (JSON)** (from `payment_service_spec.md`):

```json
{
  "transaction_id": "<uuid>",
  "tx_ref": "pay-<uuid>",
  "chapa_reference": "<reference>",
  "payer_user_id": "<string>",
  "receiver_wallet_id": "<wallet-uuid>",
  "amount": "<decimal string>",
  "currency": "ETB",
  "phone_number": "<optional; used in notification metadata>"
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

### `PUT /api/v1/wallet/:wallet_id/pay-fare`

- **Description**: atomically debits the passenger wallet, credits the driver wallet, records the transfer in Payment Service, and emits analytics/notification events. `trip_id` is required and forwarded to Payment; Trip Service active-trip validation is **not** currently enforced in code (requires `TRIP_SERVICE_BASE_URL` to be set so the trip client is configured).
- **Requires**: `TRIP_SERVICE_BASE_URL` (e.g. `http://trip:8086`). If unset, returns **502** `{ "message": "trip client not configured" }`.
- **Request (JSON)**:

```json
{
  "amount": 5,
  "driver_wallet_id": "<wallet-uuid>",
  "trip_id": "trip-uuid",
  "receiver_full_name": "Driver Name",
  "sub_city_id": "<uint from trip route; optional but forwarded to Payment for ledger>",
  "assistant_id": "<optional assistant id for notifications and Payment>",
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
  - 400 `{ "message": "amount must be > 0" }`
  - 400 `{ "message": "invalid driver_wallet_id" }`
  - 400 `{ "message": "trip_id is required" }`
  - 400 `{ "message": "receiver_full_name is required" }`
  - 400 `{ "message": "driver_wallet_id must reference a driver wallet" }`
  - 400 `{ "message": "driver wallet must not belong to the same user" }`
  - 400 `{ "message": "trip not found or not active" }` (when trip validation is enabled)
  - 400 `{ "message": "insufficient balance" }`
  - 403 `{ "message": "only passenger wallets can pay fare" }`
  - 403 `{ "message": "wallet is frozen" }`
  - 403 `{ "message": "driver wallet is frozen" }`
  - 404 `{ "message": "wallet not found" }` (passenger)
  - 404 `{ "message": "driver wallet not found" }`
  - 502 `{ "message": "trip client not configured" }`
  - 502 `{ "message": "<trip/payment service error>" }`

---

## Wallet transfer (P2P)

### `POST /api/v1/wallet/:wallet_id/transfer`

- **Description**: debits `:wallet_id` (source) and credits the receiver’s wallet atomically in the database, then records the movement via Payment Service `POST /api/v1/payments/transfers` inside the same transfer transaction (hook). Use for peer transfers where trip payment (`pay-fare`) does not apply.
- **Auth**: **`Authorization: Bearer …` required** (forwarded to Auth Service for receiver lookup).
- **Receiver resolution**: Auth Service **`GET /api/v1/auth/users/by-phone?phone=…`** (`phone` = request `to_phone_number`; Ethiopian formats `09xxxxxxxx`, `07xxxxxxxx`, or `+2519/7xxxxxxxx`). The destination wallet is the receiver’s wallet with the **same `wallet_type`** as the source wallet (e.g. passenger → passenger).
- **Receiver display name**: from Auth `display_name`; if empty, **`phone`** is used. No `receiver_full_name` in the request body.
- **Path**: `:wallet_id` — source wallet UUID.
- **Request (JSON)**:

```json
{
  "amount": 25.5,
  "to_phone_number": "0912345678",
  "message": "optional note"
}
```

- **Response 200**:

```json
{
  "success": true,
  "transaction_id": "<uuid>"
}
```

- **Errors** (non-exhaustive):
  - 400 `{ "message": "invalid json body" }`
  - 400 `{ "message": "amount must be > 0" }`
  - 400 `{ "message": "invalid to_phone_number" }`
  - 401 `{ "message": "authentication error" }`
  - 404 `{ "message": "source wallet not found" }`
  - 404 `{ "message": "receiver not found" }` (no Auth user for that phone)
  - 404 `{ "message": "destination wallet not found" }` (user exists but has no wallet of the source type)
  - 502 `{ "message": "auth client not configured" }`
  - 502 `{ "message": "<auth service error>" }`
  - 502 `{ "message": "receiver display name not available from auth" }`
  - 400 `{ "message": "invalid amount" }`
  - 400 `{ "message": "insufficient funds" }`
  - 400 `{ "message": "cannot transfer to same wallet" }`
  - 403 `{ "message": "wallet is frozen" }` (source or destination frozen)
  - 502 Payment Service errors surfaced as `{ "message": "<payment service error>" }` where applicable

---

## Assistant earnings

### `GET /api/v1/wallet/assistant/:assistantId/earnings`

- **Description**: lists Payment Service ledger rows for an assistant for a given day (`reason=fare`). **`:assistantId`** is the assistant’s Auth user id string (same as `X-User-ID`).
- **Auth**: the assistant (`X-User-ID` equals `:assistantId`) or `admin` / `superadmin`.
- **Query params**:
  - `date`: `YYYY-MM-DD` (default: today UTC)
  - `limit`: 0–200 (default 50)
  - `offset`: ≥ 0 (default 0)
- **Behavior**: proxies Payment `GET /api/v1/payments/transactions` with `assistant_id`, `reason=fare`, `date`, pagination. Requires Payment Service to support `assistant_id` (and date) filters.
- **Response 200**:

```json
{
  "assistant_id": "42",
  "date": "2026-05-04",
  "total_amount": 125.5,
  "transaction_count": 5,
  "items": [
    {
      "transaction_id": "<uuid>",
      "amount": "25.00",
      "trip_id": "<uuid>",
      "created_at": "..."
    }
  ]
}
```

- **Errors**: 400 invalid date / params; 403 forbidden; 502 payment error.

---

## Transactions (proxy)

### `GET /api/v1/wallet/transactions`

- **Description**: proxies Payment Service `GET /api/v1/payments/transactions` with restricted query params. **`sender_wallet_id` and `receiver_wallet_id` are optional**; when either is supplied, that wallet id must belong to **`X-User-ID`** or the request is **403**. When neither is supplied, Wallet resolves the caller’s wallet from **`X-User-ID`** and **`X-User-Role`** (`passenger` → passenger wallet, `driver` → driver wallet, `owner` → owner wallet) and forwards it to Payment as **`wallet_id`**. **`admin` / `superadmin`** may omit all wallet filters.
- **Headers**: **`X-User-ID` required** (gateway-injected).
- **Allowed query params**:
  - filters: `reason`, `status`, `sender_wallet_id` (optional), `receiver_wallet_id` (optional)
  - sorting: `sort`, `order`
  - pagination: `limit` (0–200), `offset` (≥ 0)
- **Forbidden query params** (set server-side when auto-resolving):
  - `payer_user_id`, `trip_id`, `wallet_id`
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
  - 401 `{ "message": "missing X-User-ID" }`
  - 403 `{ "message": "forbidden" }` (supplied wallet id does not belong to caller, or role has no wallet mapping)
  - 404 `{ "message": "wallet not found" }` (caller has no wallet for their role)
  - 400 `{ "message": "query param not supported: payer_user_id" }`
  - 400 `{ "message": "unknown query param: <name>" }`
  - 400 `{ "message": "invalid limit" }`
  - 400 `{ "message": "invalid offset" }`
  - 502 `{ "message": "<payment service error>" }`

---

## Withdraw

### `PUT /api/v1/wallet/:wallet_id/withdraw`

- **Description**: validates `bank_code` against Payment’s Chapa list, debits the **driver** or **owner** wallet, persists a local withdrawal row (with optional auto-approve / daily-limit rules from [admin configs](#admin-configs)), calls Payment `POST /api/v1/payments/withdrawals` to start the payout, reverses the debit on Payment `500`/`502`/`503`, and publishes analytics/notification events on success.
- **Headers**: **`X-User-ID` required** and must own the wallet; **`Authorization` required** for Payment calls.
- **Config keys** (optional): `daily_withdrawal_limit` (decimal string, ETB per wallet per UTC day), `auto_approve_threshold` (amounts above this are stored as `pending` locally instead of `completed`).
- **Request (JSON)**:

```json
{
  "amount": 100.0,
  "method": "bank_transfer",
  "account_name": "Abebe Kebede",
  "account_number": "1000123456789",
  "bank_code": "656",
  "withdrawal_reference": "optional-ref",
  "message": "optional note"
}
```

- **Response 200**:

```json
{
  "transaction_id": "<uuid>",
  "tx_ref": "pay-<uuid>",
  "withdrawal_reference": "<reference if any>",
  "status": "pending|succeeded|failed|cancelled"
}
```

- **Errors** (non-exhaustive):
  - 401 `{ "message": "missing X-User-ID" }`
  - 400 `{ "message": "invalid wallet id" }`
  - 400 `{ "message": "invalid json body" }`
  - 400 `{ "message": "amount must be > 0" }`
  - 400 `{ "message": "account_name, account_number, and bank_code are required" }`
  - 400 `{ "message": "invalid bank_code" }`
  - 400 `{ "message": "daily withdrawal limit exceeded" }`
  - 400 `{ "message": "owner wallet must keep minimum balance of 100 ETB" }`
  - 403 `{ "message": "forbidden" }` (not wallet owner)
  - 403 `{ "message": "wallet is frozen" }`
  - 403 `{ "message": "withdraw not allowed for this wallet type" }`
  - 404 `{ "message": "wallet not found" }`
  - 422 `{ "message": "insufficient balance" }`
  - 502 Payment errors (may reverse wallet debit on `500`/`502`/`503`)

---

## List withdrawals

### `GET /api/v1/wallet/:wallet_id/withdrawals`

- **Description**: paginated list of withdrawal records stored in the wallet database for the given wallet.
- **Query params**:
  - `limit` (default 50; values `> 0` are honored)
  - `offset` (default 0; values `> 0` are honored)
- **Response 200**:

```json
{
  "items": [
    {
      "id": "<uuid>",
      "wallet_id": "<uuid>",
      "amount": "100.00",
      "fee": "2.00",
      "net_amount": "98.00",
      "method": "bank_transfer",
      "status": "pending|completed|failed",
      "transaction_ref": "<optional>",
      "created_at": "...",
      "updated_at": "..."
    }
  ],
  "limit": 50,
  "offset": 0
}
```

- **Errors**:
  - 400 `{ "message": "invalid wallet id" }`
  - 500 `{ "message": "withdrawal repository not configured" }`
  - 500 `{ "message": "failed to list withdrawals" }`

---

## Admin: freeze

### `PUT /api/v1/wallet/:wallet_id/freeze`

- **Description**: freezes a wallet (**`admin` or `superadmin`** via gateway trust headers).
- **Headers**:
  - `X-User-ID` (**required**): acting admin user id (audit / consistency with platform)
  - `X-User-Role` (**required**): `admin` or `superadmin`
- **Response 200**:

```json
{ "success": true, "wallet_id": "<wallet-uuid>" }
```

- **Errors** (non-exhaustive):
  - 400 `{ "message": "invalid wallet id" }`
  - 401 `{ "message": "missing or invalid X-User-ID" }`
  - 403 `{ "message": "admin access required" }`
  - 404 `{ "message": "wallet not found" }`

---

## Admin: find wallets

### `GET /api/v1/wallet/admin/wallets`

- **Description**: admin wallet search with filtering, sorting, and pagination. **`superadmin`** sees all wallets; **`admin`** is scoped to wallets whose `sub_city_id` matches **`X-Sub-City`** (requires wallets to have `sub_city_id` set when data model is populated).
- **Headers**:
  - `X-User-ID` (**required**)
  - `X-User-Role` (**required**): `admin` or `superadmin`
  - `X-Sub-City` (**required** when role is `admin`)
- **Query params**:
  - **filters**:
    - `user_id` (string)
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
  - 401 `{ "message": "missing or invalid X-User-ID" }`
  - 403 `{ "message": "admin access required" }`
  - 400 `{ "message": "missing X-Sub-City" }` (role `admin`)
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

## Admin: configs

### `GET /api/v1/wallet/admin/configs`

- **Description**: returns all system configuration key/value rows (used for withdrawal limits and auto-approve threshold).
- **Headers**: `X-User-ID` and `X-User-Role` (`admin` or `superadmin`) required.
- **Response 200**:

```json
{
  "configs": [
    {
      "key": "daily_withdrawal_limit",
      "value": "5000",
      "created_at": "...",
      "updated_at": "..."
    }
  ]
}
```

- **Errors**:
  - 401 `{ "message": "missing or invalid X-User-ID" }`
  - 403 `{ "message": "admin access required" }`
  - 500 `{ "message": "config repository not configured" }`
  - 500 `{ "message": "failed to fetch configs" }`

### `PUT /api/v1/wallet/admin/configs`

- **Description**: upserts a single config key (e.g. `daily_withdrawal_limit`, `auto_approve_threshold`).
- **Headers**: same as `GET /admin/configs`.
- **Request (JSON)**:

```json
{
  "key": "daily_withdrawal_limit",
  "value": "5000"
}
```

- **Response 200**:

```json
{
  "success": true,
  "key": "daily_withdrawal_limit",
  "value": "5000"
}
```

- **Errors**:
  - 400 `{ "message": "invalid request body" }`
  - 401 `{ "message": "missing or invalid X-User-ID" }`
  - 403 `{ "message": "admin access required" }`
  - 500 `{ "message": "config repository not configured" }`
  - 500 `{ "message": "failed to update config" }`

---

## Delete wallet

### `DELETE /api/v1/wallet/:wallet_id`

- **Description**: deletes a wallet if its balance is zero
- **Response 204**: no content
- **Errors**:
  - 400 `{ "message": "invalid wallet id" }`
  - 400 `{ "message": "wallet balance must be zero" }`
  - 404 `{ "message": "wallet not found" }`
