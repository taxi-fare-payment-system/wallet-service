BEGIN;

DELETE FROM wallets WHERE user_id = '__system__' AND wallet_type = 'system';
DELETE FROM system_configs WHERE key = 'fare_platform_fee';

COMMIT;
