-- Seeds: NSE Trading Holidays for 2026 and 2027
INSERT INTO exchange_holidays (exchange, holiday, label) VALUES
  -- 2026 Holidays
  ('NSE', '2026-01-26', 'Republic Day'),
  ('NSE', '2026-03-03', 'Holi'),
  ('NSE', '2026-03-20', 'Id-Ul-Fitr (Ramzan Id)'),
  ('NSE', '2026-03-27', 'Shri Ram Navami'),
  ('NSE', '2026-04-03', 'Good Friday'),
  ('NSE', '2026-04-14', 'Dr. Baba Saheb Ambedkar Jayanti'),
  ('NSE', '2026-05-01', 'Maharashtra Day'),
  ('NSE', '2026-05-27', 'Bakri Id / Eid ul-Adha'),
  ('NSE', '2026-06-26', 'Muharram'),
  ('NSE', '2026-08-15', 'Independence Day'),
  ('NSE', '2026-09-04', 'Milad-un-Nabi (Id-e-Milad)'),
  ('NSE', '2026-10-02', 'Mahatma Gandhi Jayanti'),
  ('NSE', '2026-10-20', 'Dussehra'),
  ('NSE', '2026-11-08', 'Diwali Laxmi Pujan'),
  ('NSE', '2026-11-10', 'Diwali Balipratipada'),
  ('NSE', '2026-11-24', 'Gurunanak Jayanti'),
  ('NSE', '2026-12-25', 'Christmas'),

  -- 2027 Holidays
  ('NSE', '2027-01-26', 'Republic Day'),
  ('NSE', '2027-03-22', 'Holi'),
  ('NSE', '2027-03-26', 'Good Friday'),
  ('NSE', '2027-04-14', 'Dr. Baba Saheb Ambedkar Jayanti'),
  ('NSE', '2027-05-01', 'Maharashtra Day'),
  ('NSE', '2027-08-15', 'Independence Day'),
  ('NSE', '2027-10-02', 'Mahatma Gandhi Jayanti'),
  ('NSE', '2027-10-09', 'Dussehra'),
  ('NSE', '2027-10-28', 'Diwali Laxmi Pujan'),
  ('NSE', '2027-10-29', 'Diwali Balipratipada'),
  ('NSE', '2027-11-14', 'Gurunanak Jayanti'),
  ('NSE', '2027-12-25', 'Christmas')
ON CONFLICT (exchange, holiday) DO NOTHING;
