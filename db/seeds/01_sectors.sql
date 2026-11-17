-- Seeds: 12 Core Sectors
INSERT INTO sectors (name, benchmark_index) VALUES
  ('IT', 'NIFTY IT'),
  ('Banking', 'NIFTY BANK'),
  ('Pharma', 'NIFTY PHARMA'),
  ('Auto', 'NIFTY AUTO'),
  ('FMCG', 'NIFTY FMCG'),
  ('Energy', 'NIFTY ENERGY'),
  ('Metals', 'NIFTY METAL'),
  ('Telecom', 'NIFTY TELECOM'),
  ('Realty', 'NIFTY REALTY'),
  ('PSU Bank', 'NIFTY PSU BANK'),
  ('Media', 'NIFTY MEDIA'),
  ('Infra', 'NIFTY INFRA')
ON CONFLICT (name) DO NOTHING;
