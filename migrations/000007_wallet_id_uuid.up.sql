CREATE EXTENSION IF NOT EXISTS pgcrypto;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'wallets'
      AND column_name = 'id'
      AND udt_name = 'uuid'
  ) THEN
    RETURN;
  END IF;

  ALTER TABLE IF EXISTS wallets
      ADD COLUMN IF NOT EXISTS id_uuid UUID DEFAULT gen_random_uuid();

  UPDATE wallets
  SET id_uuid = gen_random_uuid()
  WHERE id_uuid IS NULL;

  ALTER TABLE IF EXISTS wallet_topup_credits
      ADD COLUMN IF NOT EXISTS wallet_id_uuid UUID;

  UPDATE wallet_topup_credits wtc
  SET wallet_id_uuid = w.id_uuid
  FROM wallets w
  WHERE wtc.wallet_id::TEXT = w.id::TEXT;

  ALTER TABLE IF EXISTS wallet_topup_credits
      DROP CONSTRAINT IF EXISTS wallet_topup_credits_wallet_id_fkey;

  ALTER TABLE IF EXISTS wallets
      DROP CONSTRAINT IF EXISTS wallets_pkey;

  ALTER TABLE IF EXISTS wallets
      DROP COLUMN IF EXISTS id;

  ALTER TABLE IF EXISTS wallets
      RENAME COLUMN id_uuid TO id;

  ALTER TABLE IF EXISTS wallets
      ADD CONSTRAINT wallets_pkey PRIMARY KEY (id);

  ALTER TABLE IF EXISTS wallet_topup_credits
      DROP COLUMN IF EXISTS wallet_id;

  ALTER TABLE IF EXISTS wallet_topup_credits
      RENAME COLUMN wallet_id_uuid TO wallet_id;

  ALTER TABLE IF EXISTS wallet_topup_credits
      ADD CONSTRAINT wallet_topup_credits_wallet_id_fkey
          FOREIGN KEY (wallet_id) REFERENCES wallets(id) ON DELETE RESTRICT;
END $$;
