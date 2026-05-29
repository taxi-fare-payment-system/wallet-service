BEGIN;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_enum e
    JOIN pg_type t ON e.enumtypid = t.oid
    WHERE t.typname = 'wallet_type' AND e.enumlabel = 'system'
  ) THEN
    ALTER TYPE wallet_type ADD VALUE 'system';
  END IF;
END $$;

INSERT INTO wallets (user_id, wallet_type, freezed, balance)
VALUES ('__system__', 'system', FALSE, 0)
ON CONFLICT DO NOTHING;

INSERT INTO system_configs (key, value) VALUES
    ('fare_platform_fee', '0.05')
ON CONFLICT (key) DO NOTHING;

COMMIT;
