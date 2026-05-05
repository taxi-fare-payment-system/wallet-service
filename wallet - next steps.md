# Wallet Service — Integration Next Steps

## What the service does today
- `POST /` — create a wallet (`passenger`, `driver`, `owner`); one per `(user_id, wallet_type)`
- `GET /:id` — fetch wallet by ID
- `GET /users/:userId?type=<type>` — fetch user's wallet by user ID and type
- `PUT /:wallet_id/topup` — create Chapa checkout session for passenger wallet top-up
- `POST /v1/wallet/finalize-topup` — callback from Payment Service after Chapa topup succeeds; credits wallet idempotently
- `PUT /:wallet_id/pay-fare` — atomic passenger debit + driver credit + Payment Service ledger record
- `PUT /:wallet_id/withdraw` — deduct balance + initiate Chapa bank payout via Payment Service (see Task 8)
- `GET /banks/chapa` — pass-through to Payment Service bank list (see Task 8)
- `PUT /:wallet_id/freeze` — freeze wallet (admin action, uses `X-Admin-User-Id` header)
- `GET /admin/wallets` — admin wallet search with filters
- `GET /transactions` — proxies Payment Service transaction query with restricted params

## Port
Not explicitly documented. Coordinate with the gateway team and register on port **8088**.

---

## User ID Type

`user_id` in this service is always an **integer** (auto-increment, same as Auth Service's user ID). The JWT `sub` claim is this integer as a string — parse it to integer before using it as `user_id` in any wallet operation or comparison. Never store or accept a UUID as a `user_id`.

---

## Task 1 — Expect Wallet Creation Calls from Auth Service (r-002)

Auth Service will call `POST /` after every successful user registration for `passenger` and `driver` roles. Wallet Service already supports this — ensure it is robust:

1. The `409` conflict response (`"wallet already exists for user and type"`) must be returned correctly when Auth retries wallet creation. Auth treats `409` as success.
2. The `user_id` field in the request is an **integer**. Confirm Auth Service passes the user's numeric ID (not UUID). If Auth uses UUIDs or string IDs, this is a type mismatch that must be resolved — coordinate with Auth team.
3. For `owner` role users, Auth Service should also create a wallet with `type: "owner"` at registration. Confirm this is handled.

**Expected call from Auth Service:**
```
POST http://wallet:8088/
Headers: (internal call, no JWT needed)
Body: { "user_id": 123, "type": "passenger" }
```

---

## Task 2 — Configure Trip Service Dependency for Pay-Fare (r-003)

`PUT /:wallet_id/pay-fare` requires `TRIP_SERVICE_BASE_URL` to validate the trip before executing payment. Ensure this is set:

```
TRIP_SERVICE_BASE_URL=http://trip:8086
```

The wallet service likely calls Trip Service to confirm the trip exists and is in `ACTIVE` state. If the Trip Service returns `404` or a non-ACTIVE status, pay-fare must fail with `400` or `422`.

Verify this validation is implemented. If not, add a call:
```
GET http://trip:8086/trips/<trip_id>?status=ACTIVE
Headers: Authorization: Bearer <forwarded JWT from the pay-fare caller>
```
If response is `404` or `status != ACTIVE`, return:
```json
HTTP 400
{ "message": "trip not found or not active" }
```

---

## Task 3 — Publish Analytics Events to RabbitMQ (r-002, r-008)

**Exchange:** `analytics_exchange` (topic)

Every message must include:
```json
{ "id": "<uuid-v4>", "created_at": "<RFC3339>" }
```

| Event | Routing Key | When | Additional fields |
|-------|-------------|------|-------------------|
| Wallet created | `analytics.wallet.created` | Successful `POST /` | `wallet_id`, `user_id`, `wallet_type`, `balance: 0` |
| Balance updated (topup) | `analytics.wallet.balance_updated` | After `finalize-topup` credits the wallet | `wallet_id`, `balance` (new balance), `delta` (amount added), `reason: "topup"` |
| Balance updated (fare) | `analytics.wallet.balance_updated` | After `pay-fare` debit from passenger | `wallet_id`, `balance`, `delta` (negative for debit), `reason: "fare_debit"`, `sub_city_id` |
| Balance updated (fare credit) | `analytics.wallet.balance_updated` | After `pay-fare` credit to driver | `wallet_id` (driver's), `balance`, `delta` (positive), `reason: "fare_credit"`, `sub_city_id` |
| Balance updated (withdraw) | `analytics.wallet.balance_updated` | After `withdraw` deducts balance | `wallet_id`, `balance`, `delta` (negative), `reason: "withdrawal"`, `tx_ref` |

**Environment variables to add:**
```
RABBITMQ_URL=amqp://user:pass@rabbitmq:5672/
ANALYTICS_EXCHANGE=analytics_exchange
# Exchange/queue declaration params: see next-steps/analytics.md "RabbitMQ Topology — Canonical Declaration"
```

---

## Task 4 — Publish Notification Events to RabbitMQ (r-007)

**Exchange:** `notification.exchange` (topic)

### Topup succeeded
**Routing key:** `notification.wallet.topup_succeeded`
**When:** After `finalize-topup` successfully credits the wallet.
```json
{
  "event_id": "<uuid-v4>",
  "user_id": "<payer_user_id from finalize-topup request>",
  "user_role": "passenger",
  "type": "topup_success",
  "title": "Wallet Topped Up",
  "content": "Your wallet has been credited 100.00 ETB.",
  "priority": "normal",
  "category": "billing",
  "channels": ["sms"],
  "metadata": {
    "amount": "100.00",
    "currency": "ETB",
    "transaction_id": "<uuid>"
  }
}
```

> Coordination note: Payment Service may also want to publish `notification.wallet.topup_succeeded`. Agree with the Payment team on a single publisher to avoid duplicate SMS/emails. Recommended: **Wallet Service publishes** since it has the wallet context and has just confirmed the credit.

### Fare paid (passenger notification)
**Routing key:** `notification.wallet.pay_fare_succeeded`
**When:** After `pay-fare` atomically completes.
```json
{
  "event_id": "<uuid-v4>",
  "user_id": "<passenger_user_id>",
  "user_role": "passenger",
  "type": "fare_paid",
  "title": "Fare Paid",
  "content": "You paid 50.00 ETB for your trip.",
  "priority": "normal",
  "category": "billing",
  "channels": ["push"],
  "metadata": {
    "amount": "50.00",
    "currency": "ETB",
    "trip_id": "<trip_id>",
    "transaction_id": "<uuid>",
    "assistant_id": "<assistant_user_id or null>"
  }
}
```

Include `assistant_id` in metadata if the driver has an assigned assistant (Notification Service will use this to send a separate notification to the assistant per r-007). To get `assistant_id`, call Auth Service `GET /api/v1/auth/driver/assistant` with the driver's ID — or accept `assistant_id` as an optional input to `pay-fare` (the client can pass it after resolving from the trip/driver context).

### Wallet frozen
**Routing key:** `notification.wallet.frozen`
**When:** After admin successfully freezes a wallet (`PUT /:wallet_id/freeze`).
```json
{
  "event_id": "<uuid-v4>",
  "user_id": "<wallet_owner_user_id>",
  "user_role": "passenger",
  "type": "wallet_frozen",
  "title": "Wallet Frozen",
  "content": "Your wallet has been frozen by an admin. Contact support for details.",
  "priority": "high",
  "category": "account",
  "channels": ["sms"]
}
```

To send this, you need the wallet owner's `user_id`. It's stored on the wallet object — use that.

**Environment variable to add:** `NOTIFICATION_EXCHANGE=notification.exchange`

---

## Task 5 — Replace `X-Admin-User-Id` with Header Trust (Decision 3)

The wallet freeze (`PUT /:wallet_id/freeze`) and admin wallet list (`GET /admin/wallets`) endpoints currently require a custom `X-Admin-User-Id` integer header and call Auth Service to verify admin role. This is inconsistent with every other service in the platform and is being removed.

**Replace with the standard Header Trust pattern** (same as Document and Notification services):

1. Remove all `X-Admin-User-Id` handling and the internal Auth Service call from both endpoints.
2. Read identity from the gateway-injected headers:
   - `X-User-ID` — the admin's user ID
   - `X-User-Role` — must be `admin` or `superadmin`; return `403` otherwise
3. Remove `AUTH_SERVICE_BASE_URL` dependency from Wallet Service — it is no longer needed.

**For `PUT /:wallet_id/freeze`:**
- Require `X-User-Role ∈ {admin, superadmin}`.
- Use `X-User-ID` as the actor performing the freeze (for audit purposes).

**For `GET /admin/wallets`:**
- Require `X-User-Role ∈ {admin, superadmin}`.
- Scope results to `X-Sub-City` when role is `admin`; superadmin sees all.

**Gateway update required:** Remove the old `X-Admin-User-Id` injection logic from gateway.md Task 5. The gateway treats wallet admin routes the same as any other JWT-protected route — validate JWT, inject `X-User-ID`, `X-User-Role`, `X-Sub-City`.

---

## Task 6 — Restrict `GET /users/:userId` to Own Wallet or Internal Callers (H-04)

The endpoint `GET /users/:userId?type=<type>` has no role-based restriction. Any authenticated user can query any other user's wallet. In the QR payment flow, passengers legitimately need to query the driver's wallet ID, but a passenger should not be able to query another passenger's wallet.

**Add access control:**
- If `X-User-ID == :userId` → allow (own wallet).
- If `X-User-Role ∈ {admin, superadmin}` → allow (admin lookup).
- If `X-User-Role == passenger` and `wallet_type == driver` → allow **wallet ID only** (return only `{ "wallet_id": <id> }` without balance, freeze status, or other fields). Passengers need the driver's wallet ID to execute pay-fare.
- Any other cross-user query → `403 Forbidden`.

---

## Task 7 — Add `assistant_id` to Pay-Fare and Assistant Earnings Endpoint (H-06, H-07)

### Add `assistant_id` to `PUT /:wallet_id/pay-fare`

The pay-fare request body should accept an optional `assistant_id` and a `sub_city_id`:
```json
{
  "amount": 5,
  "driver_wallet_id": 2,
  "trip_id": "trip-uuid",
  "receiver_full_name": "Driver Name",
  "assistant_id": "assistant-uuid-or-null",
  "sub_city_id": "<uuid inherited from the trip's route>"
}
```

The calling service (Trip Service) resolves `assistant_id` from Auth and reads `sub_city_id` from the trip record before calling Wallet (see Trip next-steps Tasks 4 and 7). Wallet passes `sub_city_id` to Payment `POST /transfers` (for ledger attribution). `sub_city_id` is specific to fare payments — Wallet does not populate it for any other operation (topup, withdraw, etc.).

> **No SubCity model or repository here.** Wallet Service never validates or looks up sub_city details — it only forwards the UUID it received from the caller. Auth Service owns the SubCity entity. If sub_city details are ever needed: get one with `GET http://auth:8082/api/v1/auth/subcities/:id`, list all with `GET http://auth:8082/api/v1/auth/subcities`. Both require a valid JWT.

Include `assistant_id` in the `notification.wallet.pay_fare_succeeded` event metadata so Notification Service can dispatch a separate notification to the assistant.

### Add assistant earnings query endpoint (r-011)

`GET /assistant/:assistantId/earnings`

- **Auth:** Accessible by the assistant themselves (`X-User-ID == assistantId`) or admin.
- **Query params:** `date=YYYY-MM-DD` (default: today), `limit`, `offset`.
- **Behavior:** Query Payment Service transactions where `assistant_id == assistantId` and `reason == "fare"` for the requested date.
- **Response:**
```json
{
  "assistant_id": "uuid",
  "date": "2026-05-04",
  "total_amount": 125.50,
  "transaction_count": 5,
  "items": [
    { "transaction_id": "uuid", "amount": "25.00", "trip_id": "uuid", "created_at": "..." }
  ]
}
```

This requires Payment Service to expose `GET /transactions?assistant_id=<id>` as a new filter parameter. Coordinate with the Payment team.

---

## Task 8 — Complete Withdrawal Flow with Chapa Payout (r-008)

The current `PUT /:wallet_id/withdraw` only deducts the wallet balance — it does not initiate the actual bank transfer. Payment Service now exposes two endpoints that complete this:

- `POST /withdrawals` — initiates a Chapa merchant payout and records a ledger entry as `reason = withdraw`
- `GET /banks/chapa` — returns Chapa-supported banks (cached 24 h) for `bank_code` selection

### Step 1 — Update `PUT /:wallet_id/withdraw` request body

Clients must now supply bank details alongside the amount. Update the accepted request body:

```json
{
  "amount": 100.0,
  "account_name": "Abebe Kebede",
  "account_number": "1000123456789",
  "bank_code": "656",
  "withdrawal_reference": "optional-ref",
  "message": "optional note"
}
```

- `bank_code` must be one of the codes returned by `GET /banks/chapa`. Validate before proceeding.
- The wallet owner (`X-User-ID`) is the implicit `payer_user_id`.

### Step 2 — Validate `bank_code` before deducting balance

Before touching the wallet balance, call Payment Service to confirm the `bank_code` is valid:

```
GET http://payment:8085/banks/chapa
Headers: Authorization: Bearer <forwarded JWT>
```

Check that the submitted `bank_code` exists in the returned list. If not, return `400` with `{ "message": "invalid bank_code" }`. Do not deduct balance for an invalid bank.

### Step 3 — Deduct balance, then call Payment Service to initiate payout

1. Verify wallet is not frozen and has sufficient balance. Return `422` with `{ "message": "insufficient balance" }` if not.
2. Deduct the amount from the wallet balance atomically.
3. Call Payment Service to initiate the Chapa payout:

```
POST http://payment:8085/withdrawals
Headers: Authorization: Bearer <forwarded JWT>
Body:
{
  "amount": <amount>,
  "payer_user_id": <X-User-ID as integer>,
  "account_name": "<account_name>",
  "account_number": "<account_number>",
  "bank_code": "<bank_code>",
  "withdrawal_reference": "<optional>",
  "message": "<optional>"
}
```

4. If Payment Service returns an error (`502`, `503`, `500`), **reverse the balance deduction** (add the amount back) and return the error to the client.
5. On success, return to the client:

```json
{
  "transaction_id": "<uuid>",
  "tx_ref": "pay-<uuid>",
  "withdrawal_reference": "<reference>",
  "status": "pending|succeeded|failed|cancelled"
}
```

### Step 4 — Add `GET /banks/chapa` pass-through endpoint

Expose a pass-through on Wallet Service so clients do not need to know about Payment Service:

`GET /banks/chapa`

- **Auth:** Any authenticated user (JWT required).
- **Behavior:** Forward call to `GET http://payment:8085/banks/chapa` and return the response as-is.
- Payment Service caches the Chapa bank list for 24 hours — Wallet does not need its own cache.

**Response 200:**
```json
{
  "items": [
    { "id": "1", "name": "Commercial Bank of Ethiopia", "slug": "commercial-bank-of-ethiopia", "code": "656", "currency": "ETB" }
  ]
}
```

Error `502`/`503` if Payment Service is unreachable.

### Step 5 — Publish notification event on withdrawal

After a successful Chapa payout initiation, publish to `notification.exchange`:

**Routing key:** `notification.wallet.withdrawal_initiated`
```json
{
  "event_id": "<uuid-v4>",
  "user_id": "<wallet_owner_user_id>",
  "type": "withdrawal_initiated",
  "title": "Withdrawal Initiated",
  "content": "Your withdrawal of 100.00 ETB to account ending 6789 has been initiated.",
  "priority": "normal",
  "category": "billing",
  "channels": ["sms"],
  "metadata": {
    "amount": "100.00",
    "currency": "ETB",
    "bank_code": "<code>",
    "tx_ref": "<tx_ref>",
    "transaction_id": "<uuid>"
  }
}
```

**Environment variable to add:**
```
PAYMENT_SERVICE_BASE_URL=http://payment:8085
```

---

## Task 9 — Transaction History for Users (r-008)

`GET /transactions` already proxies Payment Service. Ensure it is accessible to authenticated passengers and drivers via the gateway:

- Route: `GET /api/v1/wallet/transactions`
- The caller must filter by their own wallet ID (`sender_wallet_id` or `receiver_wallet_id`). Enforce this at the wallet service level: extract `X-User-ID` (or JWT `sub`) and verify the requested wallet belongs to that user before proxying.
- Return `403` if the user tries to query another user's wallet transactions.

---

## Summary of dependencies

| Dependency | Direction | How |
|------------|-----------|-----|
| Auth Service | Called by Wallet | Admin role verification for freeze/admin endpoints; get driver's assistant ID |
| Payment Service | Called by Wallet | `POST /initiate` for topup checkout; `POST /transfers` during pay-fare; `POST /withdrawals` + `GET /banks/chapa` during withdrawal |
| Trip Service | Called by Wallet | Validate trip is active before processing pay-fare |
| Payment Service | Calls Wallet | `POST /v1/wallet/finalize-topup` on Chapa webhook success |
| Analytics Service | Consumes events | RabbitMQ `analytics_exchange` |
| Notification Service | Consumes events | RabbitMQ `notification.exchange` |
