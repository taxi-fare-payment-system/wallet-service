# wallet_service — Interface

HTTP API exposed by the wallet service: routes, request/response shapes, auth expectations, and integration notes.

## Route index

All paths use prefix **`/api/v1/wallet`**.

| Method | Path |
| --- | --- |
| `GET` | `/healthz` |
| `GET` | `/readyz` |
| `GET` | `/banks/chapa` |
| `GET` | `/assistant/:assistantId/earnings` |
| `GET` | `/transactions` | 
| `POST` | `/` |
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

---

## Conventions

- **Base URL**: `http://<host>:<port>` (default listen port **8088**).
- **Content-Type**: `application/json` unless noted.
- **Request ID**: optional `X-Request-ID`; echoed on responses.
- **User IDs**: `user_id` values are **strings** (UUIDs from Auth). Gateway `X-User-ID` uses the same format.
- **Gateway trust headers** (JWT-validated routes): `X-User-ID`, `X-User-Role`, and for scoped admins `X-Sub-City`.
- **Outbound Auth / Payment**: caller `Authorization: Bearer …` is forwarded where noted (top-up profile, P2P receiver lookup).

**Error shape** (most non-2xx):

```json
{ "message": "..." }
```

**Wallet object**:

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "user_id": "550e8400-e29b-41d4-a716-446655440001",
  "wallet_type": "passenger",
  "freezed": false,
  "balance": "125.50",
  "created_at": "2026-03-25T12:00:00.000000000Z",
  "updated_at": "2026-03-25T12:00:00.000000000Z"
}
```

- `id`: wallet UUID.
- `wallet_type`: `passenger` | `driver` | `owner` | `system`.
- `balance`: decimal string (ETB).
- Timestamps: RFC3339Nano (UTC).

### System wallet

- One platform wallet exists: `user_id` = `__system__`, `wallet_type` = `system`.
- Created at startup / migration; not creatable via `POST /api/v1/wallet`.
- **Visible only to `superadmin`** (direct `GET`, `GET /users/...?type=system`, and `GET /admin/wallets`). Other roles receive **404** `wallet not found` for system wallets.
- Credited on each successful **pay-fare** by the configured platform fee (see [Admin configs](#admin-configs)). When the fee is > 0, a Payment `POST /api/v1/payments/transfers` record is also created (passenger wallet → system wallet) in the same atomic hook as the fare transfer.
- **Withdrawal**: only **`superadmin`** may withdraw from the system wallet, and only when the caller’s Auth account has **`totp_enabled: true`** (see Auth `GET /api/v1/auth/me`). Emits audit action `wallet.system_withdrawal_initiated`. Daily withdrawal limit / auto-approve threshold do **not** apply to system wallet withdrawals.

### Integrations (RabbitMQ)

| Exchange (env default) | Purpose |
| --- | --- |
| `analytics_exchange` (`ANALYTICS_EXCHANGE`) | `analytics.wallet.created`, `analytics.wallet.balance_updated`, audit trail `analytics.audit.recorded` |
| `notification.exchange` (`NOTIFICATION_EXCHANGE`) | wallet lifecycle / billing notifications |

Topics include: `notification.wallet.topup_succeeded`, `notification.wallet.pay_fare_succeeded`, `notification.wallet.frozen`, `notification.wallet.withdrawal_initiated`, `notification.wallet.transfer_sent`, `notification.wallet.transfer_received`.

---

## Health

### `GET /api/v1/wallet/healthz`

Liveness.

**200**: `{ "status": "ok" }`

### `GET /api/v1/wallet/readyz`

Readiness (DB ping).

**200**: `{ "status": "ok" }`  
**503**: `{ "status": "not_ready" }`

---

## Banks (Payment pass-through)

### `GET /api/v1/wallet/banks/chapa`

Proxies Payment `GET /api/v1/payments/banks/chapa`.

- **Auth**: `Authorization: Bearer …` required.

**200** (Payment shape):

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

**502**: Payment unreachable or error.

---

## Wallets

### `GET /api/v1/wallet/:wallet_id`

Fetch wallet by id.

- System wallets: **superadmin only**; others get **404**.

**200**: Wallet object  
**400**: `invalid wallet id`  
**404**: `wallet not found`

### `GET /api/v1/wallet/users/:userId?type=<wallet_type>`

Fetch a user’s wallet by id and type.

| Caller | Access |
| --- | --- |
| Own user (`X-User-ID` = `:userId`) | Full wallet |
| `admin` / `superadmin` | Full wallet |
| `passenger` + `type=driver` | `{ "wallet_id": "<uuid>" }` only (QR / pay-fare) |
| `superadmin` + `type=system` | Full system wallet |
| Other cross-user | **403** `forbidden` |

**Query**: `type` required — `passenger` | `driver` | `owner` | `system` (system: superadmin only).

**200**: Wallet object or `{ "wallet_id": "..." }`  
**400**: invalid / missing type or user id  
**403** / **404**: as above

### `POST /api/v1/wallet`

Create wallet (internal; e.g. Auth after registration). One per `(user_id, wallet_type)`.

**Request**:

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "passenger"
}
```

- `type`: `passenger` | `driver` | `owner` only (`system` rejected).

**201**: Wallet object  
**400**: invalid body / type / user id  
**409**: `wallet already exists for user and type`

---

## Top-up

### `PUT /api/v1/wallet/:wallet_id/topup`

Payment checkout for **passenger** wallet top-up.

**Auth**: `Authorization` required (forwarded to Auth `GET /api/v1/auth/me`).

**Request**:

```json
{
  "amount": 10,
  "phone_number": "+251900000000",
  "email": "user@example.com",
  "message": "optional"
}
```

- Name: `display_name` from `/me` split into first/last (both parts required).
- Phone: request `phone_number` or `/me` `phone`.

**200**:

```json
{
  "transaction_id": "<uuid>",
  "checkout_url": "<url>"
}
```

**Errors**: invalid wallet id/body/amount; **401** auth; **403** frozen / non-passenger; **404** wallet; **502** payment.

### `POST /api/v1/wallet/finalize-topup`

Payment callback; idempotent credit.

**Request**:

```json
{
  "transaction_id": "<uuid>",
  "tx_ref": "pay-<uuid>",
  "chapa_reference": "<reference>",
  "payer_user_id": "<string>",
  "receiver_wallet_id": "<wallet-uuid>",
  "amount": "10.00",
  "currency": "ETB",
  "phone_number": "<optional>"
}
```

**200**: `{ "received": true }`  
**400/500**: `{ "received": false }`

---

## Pay fare

### `PUT /api/v1/wallet/:wallet_id/pay-fare`

Atomically:

1. Debits passenger wallet: **fare + platform fee** (e.g. 20.07 ETB total when fare is 20 and fee is 0.07)
2. Credits driver wallet: **fare** only (20 ETB)
3. Credits system wallet: **platform fee** only (0.07 ETB)
4. Records one Payment transfer (passenger → driver) with `amount` = fare and optional `platform_fee` + `system_wallet_id`; Payment Service persists the passenger ledger line as **fare + fee** and a linked platform-fee credit on the system wallet (no separate passenger → system transfer)
5. Publishes analytics, audit (`wallet.fare_paid`), and notification events

**Platform fee**: config key `fare_platform_fee` (default `0.05` ETB). Superadmin updates via [Admin configs](#admin-configs).

**Requires**: Trip client configured (`TRIP_SERVICE_BASE_URL` set). Trip active validation is **not** enforced in code yet; missing trip client returns **502** `trip client not configured`.

**Request**:

```json
{
  "amount": 5,
  "driver_wallet_id": "<driver-wallet-uuid>",
  "trip_id": "trip-uuid",
  "receiver_full_name": "Driver Name",
  "sub_city_id": 1,
  "assistant_id": "<optional>",
  "message": "optional"
}
```

**200**:

```json
{
  "success": true,
  "transaction_id": "<uuid>",
  "receipt_url": "<url or null>"
}
```

**Errors** (non-exhaustive):

| Status | Message |
| --- | --- |
| 400 | `invalid wallet id`, `invalid json body`, `amount must be > 0`, `invalid driver_wallet_id`, `trip_id is required`, `receiver_full_name is required`, `driver_wallet_id must reference a driver wallet`, `driver wallet must not belong to the same user`, `insufficient balance` (fare + fee), `trip not found or not active` |
| 403 | `only passenger wallets can pay fare`, `wallet is frozen`, `driver wallet is frozen` |
| 404 | `wallet not found`, `driver wallet not found` |
| 500 | `invalid fare platform fee configuration`, `system wallet not configured` |
| 502 | `trip client not configured`, payment/trip errors |

Passenger balance must cover **fare + `fare_platform_fee`**.

**Audit metadata** (`wallet.fare_paid`): includes `platform_fee`, `system_wallet_id`, and `platform_fee_transaction_id` when a fee transfer was recorded.

---

## Wallet transfer (P2P)

### `POST /api/v1/wallet/:wallet_id/transfer`

Peer transfer between **passenger wallets only**.

- Source `:wallet_id` must be a **passenger** wallet.
- Receiver resolved via Auth `GET /api/v1/auth/users/by-phone?phone=…&role=passenger` (role fixed server-side; clients do not send `role`).
- Destination: receiver’s **passenger** wallet.
- Atomic DB transfer + Payment `POST /api/v1/payments/transfers` in hook.

**Auth**: `Authorization: Bearer …` required.

**Request**:

```json
{
  "amount": 25.5,
  "to_phone_number": "0912345678",
  "message": "optional"
}
```

**200**:

```json
{
  "success": true,
  "transaction_id": "<uuid>",
  "receipt_url": "<url or null>"
}
```

**Errors** (non-exhaustive):

| Status | Message |
| --- | --- |
| 400 | `invalid json body`, `amount must be > 0`, insufficient funds / invalid amount / same wallet |
| 401 | `authentication error` |
| 403 | `transfers are only allowed from passenger wallets`, `wallet is frozen` |
| 404 | `source wallet not found`, `no passenger account exists for this phone number; transfers are only allowed between passenger wallets`, `receiver has no passenger wallet` |
| 502 | `auth client not configured`, auth/payment errors, `receiver display name not available from auth` |

---

## Assistant earnings

### `GET /api/v1/wallet/assistant/:assistantId/earnings`

Payment ledger proxy for assistant fare collections.

- **Auth**: `X-User-ID` = `:assistantId`, or `admin` / `superadmin`.
- **Query**: `date` (`YYYY-MM-DD`, default today UTC), `limit` (0–200, default 50), `offset` (default 0).

**200**:

```json
{
  "assistant_id": "550e8400-e29b-41d4-a716-446655440000",
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

**Errors**: 400 invalid params; 403 forbidden; 502 payment error.

---

## Transactions (proxy)

### `GET /api/v1/wallet/transactions`

Proxies Payment `GET /api/v1/payments/transactions`.

**Headers**: `X-User-ID` required.

**Allowed query**: `reason`, `status`, `sender_wallet_id`, `receiver_wallet_id`, `sort`, `order`, `limit`, `offset`.

**Forbidden** (caller cannot set): `payer_user_id`, `trip_id`, `wallet_id`.

**Behavior**:

- Optional `sender_wallet_id` / `receiver_wallet_id` must belong to caller, else **403**.
- If neither set and caller is not `admin`/`superadmin`, resolves caller wallet from role (`passenger`→passenger, `driver`→driver, `owner`→owner) and sets Payment `wallet_id`.
- `admin` / `superadmin` may omit wallet filters.

**200**: Payment list shape, e.g.

```json
{
  "items": [],
  "limit": 50,
  "offset": 0,
  "sort": "created_at",
  "order": "desc"
}
```

**Errors**: 401 missing user id; 403 forbidden / no wallet for role; 404 wallet not found; 400 invalid/l unsupported params; 502 payment.

---

## Withdraw

### `PUT /api/v1/wallet/:wallet_id/withdraw`

Bank payout via Payment + local withdrawal row.

#### Driver / owner wallets

**Headers**: `X-User-ID` (must own wallet), `Authorization` (Payment).

**Config** (optional): `daily_withdrawal_limit`, `auto_approve_threshold` — see [Admin configs](#admin-configs).

#### System wallet

**Headers**: `X-User-ID`, `X-User-Role` = `superadmin`, `Authorization` (Payment + Auth `GET /me` for 2FA check).

- Caller must **not** own the wallet (`user_id` is `__system__`); access is by role instead.
- **`totp_enabled`** must be `true` on the superadmin account; otherwise **403** `two-factor authentication is required for system wallet withdrawal`.
- Non-superadmin callers receive **404** `wallet not found` (same visibility rule as other system-wallet routes).
- `daily_withdrawal_limit` and `auto_approve_threshold` are **not** applied.
- Emits audit action **`wallet.system_withdrawal_initiated`** (routing key `analytics.audit.recorded`).

Driver/owner withdrawals emit **`wallet.withdrawal_initiated`**.

#### Request (all wallet types)

```json
{
  "amount": 100,
  "method": "bank_transfer",
  "account_name": "Abebe Kebede",
  "account_number": "1000123456789",
  "bank_code": "656",
  "withdrawal_reference": "optional",
  "message": "optional"
}
```

**200**:

```json
{
  "transaction_id": "<uuid>",
  "tx_ref": "pay-<uuid>",
  "withdrawal_reference": "<optional>",
  "status": "pending|succeeded|failed|cancelled"
}
```

**Errors** (non-exhaustive):

| Status | Message |
| --- | --- |
| 401 | missing user id, `authentication error` (system wallet 2FA lookup) |
| 400 | validation, daily limit, owner minimum balance (`owner wallet must keep minimum balance of 100 ETB`) |
| 403 | `forbidden` (not wallet owner), `wallet is frozen`, `withdraw not allowed for this wallet type`, `two-factor authentication is required for system wallet withdrawal` |
| 404 | `wallet not found` (includes non-superadmin access to system wallet) |
| 422 | `insufficient balance` |
| 502 | payment error (may reverse debit on 5xx), `auth client not configured` |

---

## List withdrawals

### `GET /api/v1/wallet/:wallet_id/withdrawals`

Local withdrawal history.

**Query**: `limit` (default 50), `offset` (default 0).

**200**:

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

---

## Admin: freeze

### `PUT /api/v1/wallet/:wallet_id/freeze`

Freeze wallet (`admin` or `superadmin`).

**Headers**: `X-User-ID`, `X-User-Role` (`admin` | `superadmin`).

**200**: `{ "success": true, "wallet_id": "<uuid>" }`

**Errors**: 400 invalid id; 401 invalid/missing user id; 403 not admin; 404 not found.

---

## Admin: find wallets

### `GET /api/v1/wallet/admin/wallets`

Search wallets with filters and pagination.

- **`superadmin`**: all wallets including **system**.
- **`admin`**: scoped to `sub_city_id` = `X-Sub-City`; **system wallets excluded**.

**Headers**: `X-User-ID`, `X-User-Role`; `X-Sub-City` required for `admin`.

**Query**:

| Param | Notes |
| --- | --- |
| `user_id` | string |
| `wallet_type` | `passenger` \| `driver` \| `owner` \| `system` (`system`: superadmin only) |
| `freezed` | `true` \| `false` |
| `min_balance`, `max_balance` | decimal strings |
| `sort` | `id` (default), `balance`, `created_at`, `updated_at` |
| `order` | `desc` (default), `asc` |
| `limit` | default 50, max 200 |
| `offset` | default 0 |

**200**:

```json
{
  "items": [ /* wallet objects */ ],
  "limit": 50,
  "offset": 0,
  "sort": "id",
  "order": "desc"
}
```

---

## Admin: configs

### `GET /api/v1/wallet/admin/configs`

All system config rows (`admin` or `superadmin`).

**200**:

```json
{
  "configs": [
    {
      "key": "fare_platform_fee",
      "value": "0.05",
      "created_at": "...",
      "updated_at": "..."
    }
  ]
}
```

**Known keys**:

| Key | Purpose | Default |
| --- | --- | --- |
| `fare_platform_fee` | ETB deducted from passenger per pay-fare (credited to system wallet) | `0.05` |
| `daily_withdrawal_limit` | Max ETB withdrawn per wallet per UTC day | `5000` |
| `auto_approve_threshold` | Withdrawals above this amount stored as `pending` locally | `2000` |

### `PUT /api/v1/wallet/admin/configs`

Upsert one key.

**Request**:

```json
{
  "key": "fare_platform_fee",
  "value": "0.05"
}
```

- **`fare_platform_fee`**: **`superadmin` only**; non-negative decimal string (ETB).
- Other keys: `admin` or `superadmin`.

**200**: `{ "success": true, "key": "...", "value": "..." }`

**403**: `only superadmin can update fare platform fee`  
**400**: invalid body or invalid `fare_platform_fee` value

---

## Delete wallet

### `DELETE /api/v1/wallet/:wallet_id`

Delete wallet when balance is zero. System wallet cannot be deleted via public API (balance guard / operational policy).

**204**: no content  
**400**: `invalid wallet id`, `wallet balance must be zero`  
**404**: `wallet not found`

---

## Environment (service)

| Variable | Purpose |
| --- | --- |
| `PORT` | Listen port (default `8088`) |
| `DATABASE_URL` | PostgreSQL |
| `AUTH_SERVICE_BASE_URL` / `SERVICE_AUTH_URL` | Auth client |
| `PAYMENT_SERVICE_BASE_URL` / `SERVICE_PAYMENT_URL` | Payment client |
| `TRIP_SERVICE_BASE_URL` / `SERVICE_TRIP_URL` | Trip client (pay-fare) |
| `RABBITMQ_URL` | Event bus |
| `ANALYTICS_EXCHANGE`, `NOTIFICATION_EXCHANGE` | RabbitMQ exchange names |
| `MIGRATIONS_PATH` | SQL migrations (`file://migrations`) |
