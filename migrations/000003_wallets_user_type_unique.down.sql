BEGIN;

DROP INDEX IF EXISTS wallets_user_id_wallet_type_unique;

ALTER TABLE wallets
  ADD CONSTRAINT wallets_user_id_unique UNIQUE (user_id);

COMMIT;

