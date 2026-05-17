BEGIN;

ALTER TABLE IF EXISTS wallets
  DROP CONSTRAINT IF EXISTS wallets_user_id_unique;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM pg_tables
    WHERE schemaname = 'public'
      AND tablename = 'wallets'
  ) AND NOT EXISTS (
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

