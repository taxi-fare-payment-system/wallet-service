BEGIN;

CREATE TABLE IF NOT EXISTS wallet_topup_credits (
  payment_transaction_id UUID PRIMARY KEY,
  wallet_id              UUID NOT NULL REFERENCES wallets(id) ON DELETE RESTRICT,
  amount                 NUMERIC(12,2) NOT NULL,
  currency               TEXT NOT NULL,
  tx_ref                 TEXT NULL,
  chapa_reference        TEXT NULL,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMIT;

