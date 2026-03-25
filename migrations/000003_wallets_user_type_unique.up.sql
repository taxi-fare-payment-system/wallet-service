BEGIN;

ALTER TABLE wallets
  DROP CONSTRAINT IF EXISTS wallets_user_id_unique;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_indexes
    WHERE schemaname = 'public'
      AND indexname = 'wallets_user_id_wallet_type_unique'
  ) THEN
    CREATE UNIQUE INDEX wallets_user_id_wallet_type_unique
      ON wallets (user_id, wallet_type);
  END IF;
END $$;

COMMIT;

