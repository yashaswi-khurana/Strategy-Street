-- SS-006: Seed 6 Real Historical Corporate Actions + 1 Unverified Action for Testing

-- 1. RELIANCE: 1:1 Bonus Issue on 2017-09-07
INSERT INTO corporate_actions (stock_id, action_type, ex_date, ratio_from, ratio_to, dividend_amt, source, verified)
VALUES (
  (SELECT id FROM stocks WHERE symbol = 'RELIANCE'),
  'bonus',
  '2017-09-07',
  1.0,
  1.0,
  NULL,
  'NSE_OFFICIAL',
  TRUE
) ON CONFLICT (stock_id, action_type, ex_date) DO NOTHING;

-- 2. RELIANCE: Cash Dividend of Rs 10.00 on 2024-08-19
INSERT INTO corporate_actions (stock_id, action_type, ex_date, ratio_from, ratio_to, dividend_amt, source, verified)
VALUES (
  (SELECT id FROM stocks WHERE symbol = 'RELIANCE'),
  'dividend',
  '2024-08-19',
  NULL,
  NULL,
  10.0000,
  'NSE_OFFICIAL',
  TRUE
) ON CONFLICT (stock_id, action_type, ex_date) DO NOTHING;

-- 3. TCS: Special Dividend of Rs 28.00 on 2024-01-19
INSERT INTO corporate_actions (stock_id, action_type, ex_date, ratio_from, ratio_to, dividend_amt, source, verified)
VALUES (
  (SELECT id FROM stocks WHERE symbol = 'TCS'),
  'dividend',
  '2024-01-19',
  NULL,
  NULL,
  28.0000,
  'NSE_OFFICIAL',
  TRUE
) ON CONFLICT (stock_id, action_type, ex_date) DO NOTHING;

-- 4. INFY: 1:1 Bonus Issue on 2018-09-13
INSERT INTO corporate_actions (stock_id, action_type, ex_date, ratio_from, ratio_to, dividend_amt, source, verified)
VALUES (
  (SELECT id FROM stocks WHERE symbol = 'INFY'),
  'bonus',
  '2018-09-13',
  1.0,
  1.0,
  NULL,
  'NSE_OFFICIAL',
  TRUE
) ON CONFLICT (stock_id, action_type, ex_date) DO NOTHING;

-- 5. HDFCBANK: 1:2 Stock Split on 2019-09-19
INSERT INTO corporate_actions (stock_id, action_type, ex_date, ratio_from, ratio_to, dividend_amt, source, verified)
VALUES (
  (SELECT id FROM stocks WHERE symbol = 'HDFCBANK'),
  'split',
  '2019-09-19',
  1.0,
  2.0,
  NULL,
  'NSE_OFFICIAL',
  TRUE
) ON CONFLICT (stock_id, action_type, ex_date) DO NOTHING;

-- 6. DRREDDY: 1:5 Stock Split on 2024-10-28
INSERT INTO corporate_actions (stock_id, action_type, ex_date, ratio_from, ratio_to, dividend_amt, source, verified)
VALUES (
  (SELECT id FROM stocks WHERE symbol = 'DRREDDY'),
  'split',
  '2024-10-28',
  1.0,
  5.0,
  NULL,
  'NSE_OFFICIAL',
  TRUE
) ON CONFLICT (stock_id, action_type, ex_date) DO NOTHING;

-- 7. Unverified Action for Testing Quality Gate / Quarantine (WIPRO unverified split)
INSERT INTO corporate_actions (stock_id, action_type, ex_date, ratio_from, ratio_to, dividend_amt, source, verified)
VALUES (
  (SELECT id FROM stocks WHERE symbol = 'WIPRO'),
  'split',
  '2024-12-01',
  1.0,
  2.0,
  NULL,
  'THIRD_PARTY_UNVERIFIED_SEED',
  FALSE
) ON CONFLICT (stock_id, action_type, ex_date) DO NOTHING;

