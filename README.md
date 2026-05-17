# wallet_service

Wallet service for the taxi platform.

## Quickstart (local, Docker)

Start Postgres + wallet server:

```bash
docker compose up --build
```

Health checks:

- `GET /healthz`
- `GET /readyz`

## Local (no Docker)

1) Set env vars (see `.env`)
2) Run migrations:

```bash
go run ./cmd/migrate up
```

3) Run server:

```bash
go run ./cmd/server
```

## Environment variables

- `DATABASE_URL` (**required**): Postgres DSN
- `MIGRATIONS_PATH` (default `file://migrations`)
- `PORT` (default `8081`)
- `LOG_LEVEL` (default `info`)

Outbound HTTP:

- `PAYMENT_SERVICE_BASE_URL` (**required**)
- `HTTP_CLIENT_TIMEOUT` (default `10s`)

Optional integrations:

- `TRIP_SERVICE_BASE_URL` (if empty, pay-fare will fail trip validation)
- `TRIP_VALIDATE_PATH` (default `/validate-trip-membership`)
- `AUTH_SERVICE_BASE_URL` (if empty, freeze will return “auth service not configured”)
- `AUTH_VERIFY_ADMIN_PATH` (default `/verify-admin`)

## API examples

Create wallets:

```bash
curl -X POST http://localhost:8081/ -H "Content-Type: application/json" -d "{\"user_id\":\"1\",\"type\":\"passenger\"}"
curl -X POST http://localhost:8081/ -H "Content-Type: application/json" -d "{\"user_id\":\"1\",\"type\":\"driver\"}"
```

Get wallet by id:

```bash
curl http://localhost:8081/<wallet-uuid>
```

Get wallet by user and type:

```bash
curl "http://localhost:8081/users/1?type=passenger"
```

Top up (creates checkout in payment service):

```bash
curl -X PUT http://localhost:8081/<wallet-uuid>/topup -H "Content-Type: application/json" \
  -d "{\"amount\":10,\"phone_number\":\"+251900000000\"}"
```

Finalize topup (called by payment service):

```bash
curl -X POST http://localhost:8081/v1/wallet/finalize-topup -H "Content-Type: application/json" \
  -d "{\"transaction_id\":\"<uuid>\",\"tx_ref\":\"pay-<uuid>\",\"chapa_reference\":\"ref\",\"payer_user_id\":\"1\",\"receiver_wallet_id\":\"<wallet-uuid>\",\"amount\":\"10.00\",\"currency\":\"ETB\"}"
```

Pay fare:

```bash
curl -X PUT http://localhost:8081/<passenger-wallet-uuid>/pay-fare -H "Content-Type: application/json" \
  -d "{\"amount\":5,\"driver_wallet_id\":\"<driver-wallet-uuid>\",\"trip_id\":\"trip-uuid\",\"receiver_full_name\":\"Driver Name\"}"
```

Transactions proxy:

```bash
curl "http://localhost:8081/transactions?reason=fare&limit=50&offset=0"
```

Freeze wallet (admin-only; dummy auth):

```bash
curl -X PUT http://localhost:8081/<wallet-uuid>/freeze -H "X-Admin-User-Id: 999"
```

Withdraw:

```bash
curl -X PUT http://localhost:8081/<wallet-uuid>/withdraw -H "Content-Type: application/json" -d "{\"amount\":1}"
```

Delete wallet:

```bash
curl -X DELETE http://localhost:8081/<wallet-uuid>
```

