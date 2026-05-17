ALTER TABLE IF EXISTS wallet_topup_credits
    DROP CONSTRAINT IF EXISTS wallet_topup_credits_wallet_id_fkey;

ALTER TABLE IF EXISTS wallets
    ADD COLUMN IF NOT EXISTS id_bigint BIGSERIAL;

ALTER TABLE IF EXISTS wallet_topup_credits
    ADD COLUMN IF NOT EXISTS wallet_id_bigint BIGINT;

UPDATE wallet_topup_credits
SET wallet_id_bigint = NULL;

ALTER TABLE IF EXISTS wallets
    DROP CONSTRAINT IF EXISTS wallets_pkey;

ALTER TABLE IF EXISTS wallets
    DROP COLUMN IF EXISTS id;

ALTER TABLE IF EXISTS wallets
    RENAME COLUMN id_bigint TO id;

ALTER TABLE IF EXISTS wallets
    ADD CONSTRAINT wallets_pkey PRIMARY KEY (id);

ALTER TABLE IF EXISTS wallet_topup_credits
    DROP COLUMN IF EXISTS wallet_id;

ALTER TABLE IF EXISTS wallet_topup_credits
    RENAME COLUMN wallet_id_bigint TO wallet_id;

