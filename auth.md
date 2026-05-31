# Auth Service Interface

Base URL: `http://localhost:8080` (override with `SERVER_PORT`)

Successful JSON responses use `{ "status": "success", "message": "...", "data": ... }` (`data` may be omitted). Errors use `{ "status": "error", "message": "..." }` except login ban payload, which matches the `ACCOUNT_BANNED` shape documented under login.

## Authentication and Claims

- JWT Bearer token is required for protected endpoints.
- JWT claims include: `user_id`, `phone`, `role`, `is_verified`.
- Admin JWT includes `sub_city_id` (nullable when admin has no assignment).

## Account model (phone + role)

- A user account is uniquely identified by **`(phone, role)`**, not phone alone.
- The same phone number may have separate accounts for different roles (e.g. one `passenger` and one `driver` row).
- Duplicate registration for the same `(phone, role)` returns **409**.
- **`is_phone_verified` is per account** — verifying OTP for one role does not verify other roles on the same phone.
- Login, verify-phone, forgot-password, and reset-password all require **`role`** so the service targets the correct account.

## User object (`UserResponse`)

Returned by `GET /me`, login, register, admin user APIs, and `PATCH /me/avatar`. Example:

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "phone": "0912345678",
  "email": "user@example.com",
  "display_name": "Abebe Kebede",
  "profile_picture": "https://cdn.example/avatars/abebe.jpg",
  "role": "passenger",
  "sub_city_id": null,
  "is_active": true,
  "is_verified": true,
  "is_phone_verified": true,
  "created_at": "2026-05-16T10:00:00Z",
  "language": "en",
  "push_enabled": true,
  "biometric_enabled": false,
  "totp_enabled": false,
  "vehicle_type": "",
  "plate_number": "",
  "national_id": "",
  "license_number": "",
  "national_id_url": "",
  "license_url": "",
  "vehicle_reg_url": ""
}
```

- `profile_picture` is optional — a URL string (max 2048 characters) for the user's avatar.
- Driver/owner onboarding fields (`vehicle_type`, `plate_number`, `national_id`, `license_number`, document URLs) are populated when applicable.
- **Preferences** (also returned on `GET /me` and login):
  - `language` — UI language code (default `"en"`).
  - `push_enabled` — whether push notifications are enabled (default `true`). Update via `PATCH /preferences`.
  - `biometric_enabled` — whether biometric login is enabled (default `false`).
  - `totp_enabled` — whether TOTP authenticator-app 2FA is enabled (default `false`).

## Public Endpoints

### `GET /api/v1/auth/health`
- Health check endpoint.

### `POST /api/v1/auth/register`
- Registers a user for a specific role.
- Request (JSON or multipart form):
```json
{
  "phone": "0912345678",
  "password": "password123",
  "role": "driver",
  "display_name": "Abebe Kebede",
  "email": "user@example.com",
  "vehicle_type": "minibus",
  "plate_number": "AA-12345",
  "national_id": "ID123",
  "license_number": "DL456"
}
```
- `display_name`, `email`, and driver fields are optional unless your client flow requires them.
- Optional multipart file fields: `national_id_file`, `license_file`, `vehicle_reg_file` (uploaded via document service when configured).
- Notes:
  - Allowed `role` values: `passenger`, `driver`, `owner`, `driver-assistant`, and **`superadmin` only while no superadmin account exists yet** (bootstrap); otherwise register returns **409** `superadmin already exists`.
  - **`admin` cannot self-register**; a superadmin creates admin accounts with `POST /api/v1/auth/admin/users`.
  - Returns **409** `Phone number already registered for this role` when `(phone, role)` already has a completed account.
  - Sends OTP asynchronously through the Messaging service when `MESSAGING_SERVICE_BASE_URL` (or `MESSAGING_SERVICE_URL`) is configured. Setting `TEST_OTP_CODE` does **not** skip send.
  - Creates wallet asynchronously for `driver` and `passenger`.
  - Creates a vehicle asynchronously in the Vehicle service for new `driver` registrations when `VEHICLE_SERVICE_BASE_URL` is configured.
  - Publishes analytics and notification onboarding events.
- Response: `201` with created user profile and JWT (`LoginResponse`: `user` + `token`).

### `POST /api/v1/auth/login`
- Authenticates a specific `(phone, role)` account and returns JWT.
- Request:
```json
{
  "phone": "0912345678",
  "password": "password123",
  "role": "passenger"
}
```
- `role` is required. Allowed values: `passenger`, `driver`, `owner`, `admin`, `driver-assistant`, `superadmin`.
- When **`totp_enabled`** is `true` for the account, the response is a 2FA challenge instead of a JWT:
```json
{
  "user": { "...": "UserResponse" },
  "requires_two_factor": true,
  "two_factor_token": "<short-lived challenge token>"
}
```
- When 2FA is off, success payload includes `user` (profile fields) and `token` (JWT string).
- Returns `403` on banned users:
```json
{
  "error": "ACCOUNT_BANNED",
  "message": "Your account has been suspended. Contact support at support@example.com."
}
```

### `POST /api/v1/auth/2fa/verify-login`
- Completes login after a 2FA challenge (step 2 of the login flow). **No JWT required.**
- Request — provide **either** `code` (6-digit authenticator app code) **or** `recovery_code`:
```json
{
  "two_factor_token": "<token from login response>",
  "code": "123456"
}
```
```json
{
  "two_factor_token": "<token from login response>",
  "recovery_code": "ABCD-EFGH"
}
```
- `two_factor_token` expires after **5 minutes**.
- Response `200`: `LoginResponse` with `user` and `token` (full JWT).
- Response `401` for invalid/expired token, invalid code, or invalid recovery code.
- **Recovery codes are single-use** — a used code is removed from the account.

### `POST /api/v1/auth/2fa/recover`
- Logs in using a **recovery code** when the user no longer has access to their authenticator app. **No JWT required.** This is a single-step alternative to `login` + `verify-login` with a recovery code.
- Request:
```json
{
  "phone": "0912345678",
  "password": "password123",
  "role": "passenger",
  "recovery_code": "ABCD-EFGH"
}
```
- `role` is required (same allowed values as login).
- Validates `(phone, role)`, password, and recovery code together. Returns a generic **401** `invalid credentials or recovery code` on any mismatch (to limit enumeration).
- Response `200`: `LoginResponse` with `user` and `token` (full JWT).
- Response `400` when 2FA is not enabled on the account.
- Response `403` on banned users (same `ACCOUNT_BANNED` shape as login).
- **Recovery codes are single-use** — the redeemed code is removed from the account.

### `POST /api/v1/auth/verify-phone`
- Verifies phone OTP for a specific role account (same handler as `POST /api/v1/auth/verify-otp`).
- Request:
```json
{
  "phone": "0912345678",
  "role": "driver",
  "code": "123456"
}
```
- `role` is required. Allowed values: `passenger`, `driver`, `owner`, `driver-assistant`, `superadmin` (not `admin`).
- Verification rules:
  - If `code` equals `TEST_OTP_CODE` (when that env var is set), verification succeeds without calling the messaging service.
  - Otherwise the code is checked via `POST {MESSAGING_SERVICE_BASE_URL}/api/v1/messaging/otp/verify` (requires messaging URL to be configured).
- Response `200`: `LoginResponse` — `user` (`UserResponse`) and `token` (JWT string when the account already has a real password; empty string for in-progress sign-up with `PENDING_PASSWORD`).
- Effects:
  - Creates a placeholder row for `(phone, role)` if none exists (`password: PENDING_PASSWORD`).
  - Sets `is_phone_verified=true` **only on that role's row** (not propagated to other roles).
  - Publishes `analytics.user.phone_verified` and `notification.user.phone_verified`. When the test code is used, payloads include `verification_method: "test_otp"`.

### `POST /api/v1/auth/verify-otp`
- Alias of `POST /api/v1/auth/verify-phone` (same request body, rules, and response). Requires `phone`, `role`, and `code`.

### `POST /api/v1/auth/forgot-password`
- Sends a password-reset OTP for a specific `(phone, role)` account.
- Request:
```json
{
  "phone": "0912345678",
  "role": "passenger"
}
```
- Always returns **200** when the request is valid (including when no matching account exists, to prevent enumeration).
- OTP is sent only when an account exists for that `(phone, role)`.

### `POST /api/v1/auth/reset-password`
- Resets password after OTP verification for a specific `(phone, role)` account.
- Request:
```json
{
  "phone": "0912345678",
  "role": "passenger",
  "code": "123456",
  "new_password": "newpassword123"
}
```
- Uses the same OTP rules as verify-phone (`TEST_OTP_CODE` or messaging service).
- Response **200** on success.
- Response **400** for invalid OTP or accounts with no password set yet (`PENDING_PASSWORD`).
- Response **404** when the account is not found after OTP verification.

### `GET /api/v1/auth/users/:id/public`
- Returns basic public profile info for a user by UUID. **No JWT required.**
- Path `:id` is the user's UUID.
- Response `200` `data` shape:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "avatar": "https://cdn.example/avatars/abebe.jpg",
  "name": "Abebe Kebede",
  "role": "passenger"
}
```
- Response `404` when the user does not exist, or when the user is role **`admin`** or **`superadmin`**.

### `GET /api/v1/auth/admin/verify-admin`
- Query: `user_id` (UUID, required).
- Returns `{ "is_admin": true }` (**200**) when the user is `admin` or `superadmin`; otherwise **403** `{ "is_admin": false }`.

## Protected User Endpoints

### `GET /api/v1/auth/me`
- Returns own profile (`UserResponse`), including preferences and driver fields when present.

### `PATCH /api/v1/auth/me/avatar`
- Updates the authenticated user's profile picture URL. Any signed-in role.
- Request:
```json
{
  "profile_picture": "https://cdn.example/avatars/new.jpg"
}
```
- `profile_picture` is required in the body (max 2048 characters). Send JSON `null` to clear the avatar.
- Response `200`: updated `UserResponse`.

### `GET /api/v1/auth/users/by-phone`
- Lookup a user by phone **and role**. Requires a valid JWT (any role).
- Query:
  - `phone` (required) — Ethiopian format (`09xxxxxxxx`, `07xxxxxxxx`, or `+2519/7xxxxxxxx`).
  - `role` (required) — one of the stored role values (`passenger`, `driver`, `owner`, `admin`, `driver-assistant`, `superadmin`).
- Response `200`: same `UserResponse` shape as `GET /me`.
- Response `404` when no account exists for that exact `(phone, role)`.

### `PATCH /api/v1/auth/password`
- Changes current authenticated user's password (requires current password).

### `PATCH /api/v1/auth/preferences`
- Updates the authenticated user's preferences (partial update). Any signed-in role.
- Request — include only the fields to change:
```json
{
  "language": "am",
  "push_enabled": false,
  "biometric_enabled": true
}
```
- Response `200`: `{ "status": "success", "message": "Preferences updated successfully" }` (no `data` body).

## Two-Factor Authentication (TOTP authenticator app)

Uses [`github.com/pquerna/otp`](https://github.com/pquerna/otp) (compatible with Google Authenticator, Authy, etc.). Configure the issuer name shown in authenticator apps with `TOTP_ISSUER` (default `TaxiFare`).

### Enable flow (authenticated JWT required)

1. **`POST /api/v1/auth/2fa/setup`** — generate a TOTP secret and QR payload (does not enable 2FA yet).
   - Response `200`:
   ```json
   {
     "secret": "BASE32SECRET",
     "otpauth_url": "otpauth://totp/TaxiFare:0912345678?..."
   }
   ```
   - Scan `otpauth_url` in an authenticator app, or enter `secret` manually.

2. **`POST /api/v1/auth/2fa/enable`** — confirm with a 6-digit code from the app; enables 2FA and returns recovery codes.
   - Request: `{ "code": "123456" }`
   - Response `200`:
   ```json
   {
     "recovery_codes": ["ABCD-EFGH", "IJKL-MNOP", "..."]
   }
   ```
   - **10 recovery codes** are returned once. Store them securely; they are bcrypt-hashed in the database.

### Manage (authenticated JWT required)

- **`GET /api/v1/auth/2fa/status`** — `{ "enabled": true, "recovery_codes_left": 8 }`
- **`POST /api/v1/auth/2fa/disable`** — turn off 2FA. Requires password plus **either** authenticator `code` **or** `recovery_code`:
```json
{
  "password": "password123",
  "code": "123456"
}
```
- **`POST /api/v1/auth/2fa/recovery-codes/regenerate`** — issue new recovery codes (invalidates old ones). Requires password and authenticator `code`:
```json
{
  "password": "password123",
  "code": "123456"
}
```
- Response `200`: `{ "recovery_codes": ["...", "..."] }`

### Login with 2FA (public)

See **`POST /api/v1/auth/login`**, **`POST /api/v1/auth/2fa/verify-login`**, and **`POST /api/v1/auth/2fa/recover`** above.

## Driver Profile and Reviews (any authenticated JWT)

Path parameter `:id` is the driver's user UUID.

### `GET /api/v1/auth/drivers/:id/profile`
- Returns a driver's public profile with embedded review summary and list (includes `vehicle_type`, `plate_number`, etc.; phone omitted).
- Response `404` when the user does not exist or is not role `driver`.

### `GET /api/v1/auth/drivers/:id/reviews`
- Returns only the reviews aggregate for a driver.

### `POST /api/v1/auth/drivers/:id/reviews`
- Submits a review for a driver. The reviewer is the authenticated user (`user_id` from JWT).
- Request:
```json
{
  "message": "Very professional driver",
  "rating": 4.5
}
```
- Rules: `message` required (max 1000 chars); `rating` required, **0**–**5**; one review per reviewer per driver; cannot review yourself.
- Response **201** with the new review and updated aggregate.
- Response **409** when this reviewer has already reviewed the driver.

## Driver Assistant Management (driver/owner JWT)

### `GET /api/v1/auth/driver/assistants`
- Returns all assistant assignments linked to the authenticated driver/owner.

### `POST /api/v1/auth/driver/assistants/invite`
- Invites an assistant by **driver-assistant account phone** (resolves `(phone, driver-assistant)`).
- Request: `{ "phone": "0987654321" }`

### `PATCH /api/v1/auth/driver/assistants/:id/permissions`
- Updates permissions for an assignment owned by the caller.
- Request (all fields optional):
```json
{
  "can_collect": true,
  "can_view_earnings": true,
  "has_qr_access": true,
  "can_manage_route": false
}
```

### `DELETE /api/v1/auth/driver/assistants/:id`
- Removes an assistant assignment owned by the caller.

### Legacy driver assistant routes
- `POST /api/v1/auth/driver/assistant` — assign/link assistant (overlap with invite flow).
- `GET /api/v1/auth/driver/assistant` — get linked assistant profile.

## Assistant-Specific Endpoints (assistant JWT)

### `POST /api/v1/auth/assistant/link`
- Self-link to a driver by **driver account phone** (resolves `(phone, driver)`).
- Request: `{ "driver_phone": "0912345678" }`

### `POST /api/v1/auth/assistant/unlink`
- Assistant removes their own link to the current driver.

### `GET /api/v1/auth/assistant/driver`
- Driver profile linked to the authenticated assistant.

### `GET /api/v1/auth/assistant/info`
- The assistant's own assignment details and permissions (`can_manage_route` included when set).

## Problem Reports (any authenticated JWT)

### `POST /api/v1/auth/reports`
- Submit a support/problem report.
- Request:
```json
{
  "category": "payment",
  "description": "Optional details, max 2000 chars"
}
```

### `GET /api/v1/auth/reports`
- List the authenticated user's reports. Query: `page` (default 1), `limit` (default 20).

### `GET /api/v1/auth/reports/:id`
- Get one of the authenticated user's reports.

### Admin report management (`admin` / `superadmin` JWT)
- `GET /api/v1/auth/admin/reports` — list all reports; optional query `status`, `page`, `limit`.
- `GET /api/v1/auth/admin/reports/:id`
- `PATCH /api/v1/auth/admin/reports/:id/status` — body:
```json
{
  "status": "under_review",
  "notes": "optional admin notes"
}
```
- Allowed `status`: `under_review`, `resolved`, `rejected`, `escalated`.

## Vehicle Change Requests (driver JWT)

### `POST /api/v1/auth/vehicle-change-requests`
- Submit a vehicle change request.
- Optional query params: `vehicle_id`, `old_plate`, `old_type` (current vehicle context).
- Request:
```json
{
  "reason_category": "change_vehicle",
  "new_plate_number": "AA-99999",
  "new_vehicle_type": "minibus",
  "description": "Optional, max 2000 chars"
}
```
- `reason_category`: `fix_mistake` or `change_vehicle`.

### `GET /api/v1/auth/vehicle-change-requests`
- List the authenticated driver's requests. Query: `page`, `limit`.

### `GET /api/v1/auth/vehicle-change-requests/:id`
- Get one of the authenticated driver's requests.

### Admin vehicle change management
- `GET /api/v1/auth/admin/vehicle-change-requests`
- `GET /api/v1/auth/admin/vehicle-change-requests/:id`
- `PATCH /api/v1/auth/admin/vehicle-change-requests/:id/status` — body:
```json
{
  "status": "approved",
  "notes": "optional"
}
```
- Allowed `status`: `under_review`, `approved`, `rejected`.
- Optional query on status update: `vehicle_service_url`.

## Route Change Requests (driver JWT)

### `POST /api/v1/auth/route-change-requests`
- Submit a route change request.
- Optional query: `old_route_id`.
- Request:
```json
{
  "new_route_id": "route-uuid-or-id",
  "new_route_name": "Bole → Megenagna",
  "reason": "Optional, max 2000 chars",
  "transport_doc_url": "https://..."
}
```

### `GET /api/v1/auth/route-change-requests`
- List the authenticated driver's requests. Query: `page`, `limit`.

### `GET /api/v1/auth/route-change-requests/:id`
- Get one of the authenticated driver's requests.

### Admin route change management
- `GET /api/v1/auth/admin/route-change-requests`
- `GET /api/v1/auth/admin/route-change-requests/:id`
- `PATCH /api/v1/auth/admin/route-change-requests/:id/status` — same status values as vehicle change requests.

## Admin Endpoints (Admin/Superadmin JWT)

### User Management
- `GET /api/v1/auth/admin/users`
- `GET /api/v1/auth/admin/users/:id`
- `POST /api/v1/auth/admin/users` — JWT role **`admin`** may only create **`owner`** users; **`superadmin`** may create any valid role. Uniqueness is on `(phone, role)`. New `admin` / `superadmin` users get `is_phone_verified=true` by default. Optional `sub_city_id` applies only when `role` is `admin`.
- `PATCH /api/v1/auth/admin/users/:id` — changing `phone` or `role` checks `(phone, role)` conflicts.
- `DELETE /api/v1/auth/admin/users/:id`
- `GET /api/v1/auth/admin/users/role/:role`

### Verification
- `GET /api/v1/auth/admin/pending-drivers` — unverified drivers, owners, and driver-assistants.
- `POST /api/v1/auth/admin/verify-driver` — body: `{ "user_id": "<uuid>" }`
- `POST /api/v1/auth/admin/unverify-driver` — same body.

On driver verification, the service may create a QR code (QR service) and assign the driver's vehicle (Vehicle service) when those URLs are configured.

### Ban and Unban
- `POST /api/v1/auth/admin/users/:id/ban` — body: `{ "reason": "..." }`
- `POST /api/v1/auth/admin/users/:id/unban`

### Admin create user request (reference)
```json
{
  "phone": "0912345678",
  "password": "password123",
  "role": "admin",
  "display_name": "Optional Name",
  "profile_picture": "https://cdn.example/avatars/admin.jpg",
  "sub_city_id": 1,
  "is_active": true,
  "is_verified": true
}
```
- Sub-city keys accepted on create: `sub_city_id` (preferred), `subcity_id`, or `subCityId` — only when `role` is `admin`.

## SubCity Endpoints

### Canonical routes (`/api/v1/auth/subcities`)
- `GET /api/v1/auth/subcities` — list (any authenticated JWT).
- `GET /api/v1/auth/subcities/:id`
- `POST /api/v1/auth/subcities` — `{ "name": "Bole" }` (**superadmin**).
- `PATCH /api/v1/auth/subcities/:id` — partial update (**superadmin**).
- `DELETE /api/v1/auth/subcities/:id` (**superadmin**).
- `POST /api/v1/auth/subcities/:id/admins/:userId` — assign admin (**superadmin**).
- `DELETE /api/v1/auth/subcities/:id/admins/:userId` — unassign admin (**superadmin**).

## Internal Endpoints (service-to-service; no public gateway exposure)

### `GET /api/v1/auth/internal/users/:id`
- Full `UserResponse` for a user by UUID.

### `GET /api/v1/auth/drivers/:id/assistant`
- Assistant assignment for a specific driver.

### `GET /internal/drivers/:driver_id/assistant/permissions`
- Permission flags for the driver's assigned assistant.

### `GET /internal/users/:id/contact`
- Contact info: `phone`, `email`, `display_name`, `full_name`.

### `GET /internal/users/:id/driver`
- Driver assignment info for an assistant user ID.

## Async Integrations

### Messaging Service
- `POST {MESSAGING_SERVICE_BASE_URL}/api/v1/messaging/otp/send` — after register (body: `{ "recipient": "<phone>", "type": "sms" }`).
- `POST {MESSAGING_SERVICE_BASE_URL}/api/v1/messaging/otp/verify` — on verify-phone / verify-otp / reset-password unless `TEST_OTP_CODE` matches.
- Leave `TEST_OTP_CODE` unset in production.

### Wallet Service
- `POST {WALLET_SERVICE_BASE_URL}/api/v1/wallet`
```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "driver"
}
```

### Vehicle Service
- On driver register: `POST {VEHICLE_SERVICE_BASE_URL}/api/v1/vehicles`
- On driver verification: list owner vehicles, approve if pending, assign driver to vehicle.

### QR Service
- On successful driver verification: `POST {QR_SERVICE_BASE_URL}/api/v1/qr` with `{ "driver_id": "<uuid>" }`.

### RabbitMQ
- Analytics exchange: `RABBITMQ_EXCHANGE_ANALYTICS` (default `analytics_events`)
- Notification exchange: `RABBITMQ_EXCHANGE_NOTIFICATION` (default `notification_events`)

Published topic examples:
- `analytics.user.created`
- `analytics.user.status_updated`
- `analytics.user.phone_verified`
- `analytics.user.banned` / `analytics.user.unbanned`
- `notification.user.welcome`
- `notification.user.phone_verified`
- `notification.user.driver_pending_documents`
- `notification.user.driver_verified`
- `notification.user.banned` / `notification.user.unbanned`

## Observability
- Every HTTP request receives `X-Request-ID` (UUID) in the response.
- Structured logs tag activity with `direction` (`inbound` / `outbound`), `component` (`http`, `messaging`, `rabbitmq`, `database`), and `request_id` when available.

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `SERVER_PORT` | HTTP port (default `8080`) |
| `JWT_SECRET` | JWT signing secret |
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` | PostgreSQL |
| `TEST_OTP_CODE` | Dev-only OTP bypass on verify-phone / reset-password |
| `MESSAGING_SERVICE_BASE_URL` or `MESSAGING_SERVICE_URL` | OTP send/verify |
| `WALLET_SERVICE_BASE_URL` or `WALLET_SERVICE_URL` | Wallet creation |
| `QR_SERVICE_BASE_URL` or `QR_SERVICE_URL` | Driver QR on verification |
| `VEHICLE_SERVICE_BASE_URL` or `VEHICLE_SERVICE_URL` | Vehicle create/assign |
| `ROUTE_SERVICE_BASE_URL` or `ROUTE_SERVICE_URL` | Route change workflow |
| `TRIP_SERVICE_BASE_URL` or `TRIP_SERVICE_URL` | Trip-related integrations |
| `RABBITMQ_URL` | Message broker |
| `RABBITMQ_EXCHANGE_ANALYTICS` | Analytics exchange name |
| `RABBITMQ_EXCHANGE_NOTIFICATION` | Notification exchange name |
| `APPEAL_CONTACT` | Shown in ban messages |
| `TOTP_ISSUER` | Name shown in authenticator apps when scanning 2FA QR (default `TaxiFare`) |
