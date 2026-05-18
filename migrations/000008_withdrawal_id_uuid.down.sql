-- Revert withdrawals UUID migration back to BIGSERIAL/BIGINT
DROP TABLE IF EXISTS withdrawals;

CREATE TABLE withdrawals (
    id BIGSERIAL PRIMARY KEY,
    wallet_id BIGINT NOT NULL,
    amount NUMERIC(12, 2) NOT NULL,
    fee NUMERIC(12, 2) NOT NULL DEFAULT 0,
    net_amount NUMERIC(12, 2) NOT NULL,
    method VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    transaction_ref VARCHAR(100),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_wallet
      FOREIGN KEY(wallet_id)
      REFERENCES wallets(id)
      ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_withdrawals_transaction_ref ON withdrawals (transaction_ref) WHERE transaction_ref IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_withdrawals_wallet_id ON withdrawals (wallet_id);
