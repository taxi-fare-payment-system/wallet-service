# Auth Service Interface

Base URL: `http://localhost:8082`

Successful JSON responses use `{ "status": "success", "message": "...", "data": ... }` (`data` may be omitted). Errors use `{ "status": "error", "message": "..." }` except login ban payload, which matches the `ACCOUNT_BANNED` shape documented under login.

## Authentication and Claims
- JWT Bearer token is required for protected endpoints.
- JWT claims include: `user_id`, `phone`, `role`, `is_verified`.
- Admin JWT now includes `sub_city_id` (nullable when admin has no assignment).

## Public Endpoints

### `GET /api/v1/auth/health`
- Health check endpoint.

### `POST /api/v1/auth/register`
- Registers a user.
- Request:
```json
{
  "phone": "0912345678",
  "password": "password123",
  "role": "driver",
  "display_name": "Abebe Kebede"
}
```
- Notes:
  - Allowed `role` values: `passenger`, `driver`, `owner`, `driver-assistant`, and **`superadmin` only while no superadmin account exists yet** (bootstrap); otherwise register returns **409** `superadmin already exists`.
  - **`admin` cannot self-register**; a superadmin creates admin accounts with `POST /api/v1/auth/admin/users`.
  - Sends OTP asynchronously through Messaging service (skipped when `TEST_OTP_CODE` is set, or when `MESSAGING_SERVICE_BASE_URL` is empty).
  - Creates wallet asynchronously for `driver` and `passenger`.
  - Publishes analytics and notification onboarding events.
- Response: `201` with created user profile (same shape as `GET /me` user object).

### `POST /api/v1/auth/login`
- Authenticates user and returns JWT.
- Request: `phone`, `password` (JSON).
- Success payload includes `user` (profile fields) and `token` (JWT string).
- Returns `403` on banned users:
```json
{
  "error": "ACCOUNT_BANNED",
  "message": "Your account has been suspended. Contact support at support@example.com."
}
```

### `POST /api/v1/auth/verify-phone`
- Verifies phone OTP.
- Request:
```json
{
  "phone": "0912345678",
  "code": "123456"
}
```
- Effects:
  - Marks `is_phone_verified=true`.
  - Publishes `analytics.user.phone_verified` and `notification.user.phone_verified` on every successful verification (including when the code matches `TEST_OTP_CODE`). When the test code is used, payloads include `verification_method: "test_otp"` (analytics body and notification metadata).

## Protected User Endpoints

### `GET /api/v1/auth/me`
- Returns own profile including `display_name`, `is_phone_verified`, and `email` if present.

### `PATCH /api/v1/auth/password`
- Changes current authenticated user's password.

## Driver Assistant Endpoints (Driver JWT only)

### `POST /api/v1/auth/driver/assistant`
- Assign assistant to the authenticated driver.
- Request:
```json
{
  "assistant_user_id": 5
}
```
- Rules:
  - Assistant must be role `driver-assistant`.
  - Assistant must be verified.
  - One assistant per driver.

### `GET /api/v1/auth/driver/assistant`
- Returns current assistant assignment.
- `404` when not assigned.

### `DELETE /api/v1/auth/driver/assistant`
- Unassigns current assistant.
- Returns `204`.

## Admin Endpoints (Admin/Superadmin JWT)

### User Management
- `GET /api/v1/auth/admin/users`
- `GET /api/v1/auth/admin/users/:id`
- `POST /api/v1/auth/admin/users` (JWT role **`admin`** may only create users with role **`owner`** — any other `role` returns **403**; **`superadmin`** may create any valid role. New `admin` or `superadmin` users get `is_phone_verified=true` by default. Optional `sub_city_id` is applied only when the new user’s `role` is `admin` and must reference an existing sub-city; if `role` is not `admin`, `sub_city_id` is ignored.)
- `PATCH /api/v1/auth/admin/users/:id`
- `DELETE /api/v1/auth/admin/users/:id`
- `GET /api/v1/auth/admin/users/role/:role`

### Verification
- `GET /api/v1/auth/admin/pending-drivers` — unverified drivers, owners, and driver-assistants pending approval.
- `POST /api/v1/auth/admin/verify-driver` — body:
```json
{ "user_id": "550e8400-e29b-41d4-a716-446655440000" }
```
(`user_id` must be a quoted UUID string from `GET /admin/pending-drivers` or user list responses.)
- `POST /api/v1/auth/admin/unverify-driver` — same body as verify-driver (`user_id`).

### Ban and Unban
- `POST /api/v1/auth/admin/users/:id/ban`
  - Request:
```json
{
  "reason": "Violated terms of service"
}
```
  - Response includes: `user_id`, `banned`, `reason`, `banned_by`, `banned_at`.
- `POST /api/v1/auth/admin/users/:id/unban`
  - Response includes: `user_id`, `banned`, `unbanned_by`, `unbanned_at`.

### Admin create user request (reference)
```json
{
  "phone": "0912345678",
  "password": "password123",
  "role": "admin",
  "display_name": "Optional Name",
  "sub_city_id": 1,
  "is_active": true,
  "is_verified": true
}
```
- For sub-city on create, use **`sub_city_id`** (preferred), or **`subcity_id`** / **`subCityId`**; only the first non-null among those three is used. This applies only when **`role` is `admin`**.
- `sub_city_id` / `subcity_id` / `subCityId`, `is_active`, `is_verified`, and `display_name` are optional where not required by validation.

### Admin update user (reference)
`PATCH /api/v1/auth/admin/users/:id` — all fields optional; include only what changes:
```json
{
  "phone": "0912000000",
  "password": "newpassword",
  "role": "driver",
  "display_name": "Updated",
  "is_active": true,
  "is_verified": false
}
```

## SubCity Endpoints (Auth is owner)

### Canonical routes (`/api/v1/auth/subcities`, superadmin write)
- `GET /api/v1/auth/subcities` — list all sub-cities with assigned admins (any authenticated JWT).
- `GET /api/v1/auth/subcities/:id` — one sub-city with assigned admins.
- `POST /api/v1/auth/subcities` — body `{ "name": "Bole" }` (superadmin).
- `PUT /api/v1/auth/subcities/:id` — body JSON must include `"name": "..."` (superadmin); omitting `name` returns `400` (`no fields to update`).
- `DELETE /api/v1/auth/subcities/:id` (superadmin).
- `POST /api/v1/auth/subcities/:id/admins/:userId` — assign admin user `userId` to sub-city `:id` (path param, no JSON body).
- `DELETE /api/v1/auth/subcities/:id/admins/:userId` — remove that assignment.

### Legacy compatibility (`/api/v1/auth/admin/subcities/...`, superadmin only)
Same behaviors as above, different paths and verbs where noted:

- `GET /api/v1/auth/admin/subcities/assignment/check` — query: `subcity_id`, `admin_user_id` (both required). Response indicates whether that admin is assigned to that sub-city.
- `GET /api/v1/auth/admin/subcities` — list (includes assigned admins per row).
- `POST /api/v1/auth/admin/subcities` — body `{ "name": "..." }`.
- `GET /api/v1/auth/admin/subcities/:id` — get one.
- `GET /api/v1/auth/admin/subcities/:id/admin-assignment` — admins assigned to `:id` plus assignment flags.
- `PATCH /api/v1/auth/admin/subcities/:id` — partial update; body must include `name` (handler rejects empty update).
- `DELETE /api/v1/auth/admin/subcities/:id`
- `POST /api/v1/auth/admin/subcities/:id/assign-admin` — body `{ "admin_user_id": 7 }`.
- `POST /api/v1/auth/admin/subcities/:id/unassign-admin` — body `{ "admin_user_id": 7 }`.

## Internal Endpoints (No public gateway exposure)

### `GET /api/v1/auth/drivers/:id/assistant`
- Returns assistant assignment for a specific driver.

### `GET /internal/users/:id/contact`
- Returns user contact info (includes `display_name` for Wallet / Payment attribution; inter-service).
```json
{
  "phone": "+251911223344",
  "email": "user@example.com",
  "display_name": "Recipient Name"
}
```

## Async Integrations

### Messaging Service
- `POST {MESSAGING_SERVICE_BASE_URL}/api/v1/messaging/otp/send` (not called when `TEST_OTP_CODE` is set, or when the messaging base URL is empty)
- `POST {MESSAGING_SERVICE_BASE_URL}/api/v1/messaging/otp/verify` (skipped when the submitted code equals `TEST_OTP_CODE`; leave `TEST_OTP_CODE` unset in production)

### Wallet Service
- `POST {WALLET_SERVICE_BASE_URL}/api/v1/wallet`
- Body:
```json
{
  "user_id": 123,
  "type": "driver"
}
```

### QR Service
- Triggered on successful driver verification.
- `POST {QR_SERVICE_BASE_URL}/api/v1/qr`
- Body:
```json
{
  "driver_id": "123"
}
```

### RabbitMQ Exchanges
- Analytics exchange: `analytics_exchange`
- Notification exchange: `notification.exchange`

Published topic examples:
- `analytics.user.created`
- `analytics.user.status_updated`
- `analytics.user.phone_verified`
- `analytics.user.banned`
- `analytics.user.unbanned`
- `notification.user.welcome`
- `notification.user.phone_verified`
- `notification.user.driver_pending_documents`
- `notification.user.driver_verified`
- `notification.user.banned`
- `notification.user.unbanned`

## Environment Variables
- `TEST_OTP_CODE` (optional; dev-only wildcard OTP for verify-phone; when set, SMS send on register is skipped)
- `MESSAGING_SERVICE_BASE_URL`
- `WALLET_SERVICE_BASE_URL`
- `QR_SERVICE_BASE_URL`
- `RABBITMQ_URL`
- `ANALYTICS_EXCHANGE` (default `analytics_exchange`)
- `NOTIFICATION_EXCHANGE` (default `notification.exchange`)
- `APPEAL_CONTACT`
