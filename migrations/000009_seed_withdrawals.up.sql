-- Seed withdrawal records for testing
-- Wallet ID: ca9bdf57-6406-4c02-851a-1cdb448a4c73

INSERT INTO withdrawals (id, wallet_id, amount, fee, net_amount, method, status, transaction_ref, created_at, updated_at)
VALUES
  (gen_random_uuid(), 'ca9bdf57-6406-4c02-851a-1cdb448a4c73', 2000.00, 10.00, 1990.00, 'telebirr', 'completed', 'TB-2026-001', NOW() - INTERVAL '1 day', NOW() - INTERVAL '1 day'),
  (gen_random_uuid(), 'ca9bdf57-6406-4c02-851a-1cdb448a4c73', 500.00, 5.00, 495.00, 'cbe_birr', 'completed', 'CB-2026-002', NOW() - INTERVAL '3 days', NOW() - INTERVAL '3 days'),
  (gen_random_uuid(), 'ca9bdf57-6406-4c02-851a-1cdb448a4c73', 1500.00, 15.00, 1485.00, 'bank', 'pending', NULL, NOW() - INTERVAL '6 hours', NOW() - INTERVAL '6 hours'),
  (gen_random_uuid(), 'ca9bdf57-6406-4c02-851a-1cdb448a4c73', 3000.00, 25.00, 2975.00, 'telebirr', 'completed', 'TB-2026-004', NOW() - INTERVAL '7 days', NOW() - INTERVAL '7 days'),
  (gen_random_uuid(), 'ca9bdf57-6406-4c02-851a-1cdb448a4c73', 800.00, 8.00, 792.00, 'telebirr', 'failed', NULL, NOW() - INTERVAL '10 days', NOW() - INTERVAL '10 days')
ON CONFLICT DO NOTHING;
