-- Remove seed withdrawal records
DELETE FROM withdrawals WHERE transaction_ref IN ('TB-2026-001', 'CB-2026-002', 'TB-2026-004');
DELETE FROM withdrawals WHERE amount IN (1500.00, 800.00) AND wallet_id = 'ca9bdf57-6406-4c02-851a-1cdb448a4c73';
