DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = 'public'
      AND table_name = 'withdrawals'
  ) THEN
    RETURN;
  END IF;

  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'withdrawals'
      AND column_name = 'id'
      AND udt_name = 'uuid'
  ) THEN
    RETURN;
  END IF;

  ALTER TABLE withdrawals DROP CONSTRAINT IF EXISTS fk_wallet;
  DROP INDEX IF EXISTS idx_withdrawals_wallet_id;

  ALTER TABLE withdrawals ADD COLUMN IF NOT EXISTS id_new UUID DEFAULT gen_random_uuid();
  ALTER TABLE withdrawals ADD COLUMN IF NOT EXISTS wallet_id_new UUID;

  UPDATE withdrawals w
  SET wallet_id_new = wl.id
  FROM wallets wl
  WHERE w.wallet_id::TEXT = wl.id::TEXT;

  ALTER TABLE withdrawals DROP COLUMN id;
  ALTER TABLE withdrawals DROP COLUMN wallet_id;

  ALTER TABLE withdrawals RENAME COLUMN id_new TO id;
  ALTER TABLE withdrawals RENAME COLUMN wallet_id_new TO wallet_id;

  ALTER TABLE withdrawals ADD PRIMARY KEY (id);
  ALTER TABLE withdrawals ALTER COLUMN wallet_id SET NOT NULL;
  ALTER TABLE withdrawals ADD CONSTRAINT fk_withdrawals_wallet
    FOREIGN KEY (wallet_id) REFERENCES wallets(id) ON DELETE CASCADE;

  CREATE INDEX IF NOT EXISTS idx_withdrawals_wallet_id ON withdrawals (wallet_id);
  CREATE UNIQUE INDEX IF NOT EXISTS idx_withdrawals_transaction_ref ON withdrawals (transaction_ref) WHERE transaction_ref IS NOT NULL;
END $$;
