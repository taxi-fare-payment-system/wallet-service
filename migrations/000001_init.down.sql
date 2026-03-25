BEGIN;

DROP TABLE IF EXISTS wallets;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_type WHERE typname = 'wallet_type') THEN
    DROP TYPE wallet_type;
  END IF;
END $$;

COMMIT;

