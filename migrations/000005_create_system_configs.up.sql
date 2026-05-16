CREATE TABLE IF NOT EXISTS system_configs (
    key VARCHAR(100) PRIMARY KEY,
    value TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO system_configs (key, value) VALUES
    ('daily_withdrawal_limit', '5000'),
    ('auto_approve_threshold', '2000')
ON CONFLICT (key) DO NOTHING;
