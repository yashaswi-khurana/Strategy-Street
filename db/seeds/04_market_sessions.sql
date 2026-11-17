-- Seeds: Muhurat / Special Trading Sessions
INSERT INTO market_sessions (exchange, session_date, open_time, close_time, label) VALUES
  ('NSE', '2026-11-08', '18:15:00', '19:15:00', 'muhurat'),
  ('NSE', '2027-10-28', '18:15:00', '19:15:00', 'muhurat')
ON CONFLICT (exchange, session_date) DO NOTHING;
