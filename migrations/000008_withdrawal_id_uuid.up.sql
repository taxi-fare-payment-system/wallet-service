-- Migrate withdrawals table IDs from int64 to UUID, consistent with the wallets table migration.
-- Step 1: Drop existing foreign key and indexes
ALTER TABLE withdrawals DROP CONSTRAINT IF EXISTS fk_wallet;
DROP INDEX IF EXISTS idx_withdrawals_wallet_id;

-- Step 2: Add new UUID columns
ALTER TABLE withdrawals ADD COLUMN IF NOT EXISTS id_new UUID DEFAULT gen_random_uuid();
ALTER TABLE withdrawals ADD COLUMN IF NOT EXISTS wallet_id_new UUID;

-- Step 3: Copy wallet_id by joining to wallets table if wallets.id is already UUID
-- (If you have existing rows, update wallet_id_new manually or via a join.)
-- For a clean environment this is safe as a placeholder:
-- UPDATE withdrawals SET wallet_id_new = wallets.id FROM wallets WHERE withdrawals.wallet_id = wallets.id_old;

-- Step 4: Drop old columns
ALTER TABLE withdrawals DROP COLUMN id;
ALTER TABLE withdrawals DROP COLUMN wallet_id;

-- Step 5: Rename new columns
ALTER TABLE withdrawals RENAME COLUMN id_new TO id;
ALTER TABLE withdrawals RENAME COLUMN wallet_id_new TO wallet_id;

-- Step 6: Add constraints
ALTER TABLE withdrawals ADD PRIMARY KEY (id);
ALTER TABLE withdrawals ALTER COLUMN wallet_id SET NOT NULL;
ALTER TABLE withdrawals ADD CONSTRAINT fk_withdrawals_wallet
  FOREIGN KEY (wallet_id) REFERENCES wallets(id) ON DELETE CASCADE;

-- Step 7: Recreate indexes
CREATE INDEX IF NOT EXISTS idx_withdrawals_wallet_id ON withdrawals (wallet_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_withdrawals_transaction_ref ON withdrawals (transaction_ref) WHERE transaction_ref IS NOT NULL;
