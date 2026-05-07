BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'wallet_type') THEN
    CREATE TYPE wallet_type AS ENUM ('passenger', 'driver', 'owner');
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS wallets (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      TEXT NOT NULL,
  wallet_type  wallet_type NOT NULL,
  freezed      BOOLEAN NOT NULL DEFAULT FALSE,
  balance      NUMERIC(12,2) NOT NULL DEFAULT 0,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT wallets_user_id_unique UNIQUE (user_id),
  CONSTRAINT wallets_balance_non_negative CHECK (balance >= 0)
);

COMMIT;

