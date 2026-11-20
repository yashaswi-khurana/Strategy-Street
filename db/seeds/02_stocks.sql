-- Seeds: ~50 Core NSE Stocks with real ISINs and tick sizes (0.01 for sub-250 stocks, 0.05 otherwise)
INSERT INTO stocks (instrument_key, symbol, company_name, exchange, sector_id, lot_size, tick_size, is_active, listing_date) VALUES
  -- IT
  ('NSE_EQ|INE467B01029', 'TCS', 'Tata Consultancy Services Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'IT'), 1, 0.05, TRUE, '2004-08-25'),
  ('NSE_EQ|INE009A01021', 'INFY', 'Infosys Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'IT'), 1, 0.05, TRUE, '1993-06-14'),
  ('NSE_EQ|INE075A01022', 'WIPRO', 'Wipro Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'IT'), 1, 0.05, TRUE, '1995-11-08'),
  ('NSE_EQ|INE860A01027', 'HCLTECH', 'HCL Technologies Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'IT'), 1, 0.05, TRUE, '2000-01-11'),
  ('NSE_EQ|INE669C01036', 'TECHM', 'Tech Mahindra Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'IT'), 1, 0.05, TRUE, '2006-08-28'),
  ('NSE_EQ|INE214T01019', 'LTIM', 'LTIMindtree Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'IT'), 1, 0.05, TRUE, '2016-07-21'),

  -- Banking
  ('NSE_EQ|INE040A01034', 'HDFCBANK', 'HDFC Bank Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Banking'), 1, 0.05, TRUE, '1995-11-08'),
  ('NSE_EQ|INE090A01021', 'ICICIBANK', 'ICICI Bank Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Banking'), 1, 0.05, TRUE, '1997-09-17'),
  ('NSE_EQ|INE237A01028', 'KOTAKBANK', 'Kotak Mahindra Bank Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Banking'), 1, 0.05, TRUE, '2003-02-06'),
  ('NSE_EQ|INE238A01034', 'AXISBANK', 'Axis Bank Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Banking'), 1, 0.05, TRUE, '1998-11-16'),
  ('NSE_EQ|INE095A01012', 'INDUSINDBK', 'IndusInd Bank Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Banking'), 1, 0.05, TRUE, '1998-03-25'),
  ('NSE_EQ|INE296A01024', 'BAJFINANCE', 'Bajaj Finance Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Banking'), 1, 0.05, TRUE, '2003-04-01'),
  ('NSE_EQ|INE918I01018', 'BAJAJFINSV', 'Bajaj Finserv Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Banking'), 1, 0.05, TRUE, '2008-05-26'),

  -- Pharma
  ('NSE_EQ|INE044A01036', 'SUNPHARMA', 'Sun Pharmaceutical Industries Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Pharma'), 1, 0.05, TRUE, '1995-02-08'),
  ('NSE_EQ|INE089A01023', 'DRREDDY', 'Dr. Reddy''s Laboratories Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Pharma'), 1, 0.05, TRUE, '2001-04-11'),
  ('NSE_EQ|INE059A01026', 'CIPLA', 'Cipla Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Pharma'), 1, 0.05, TRUE, '1995-02-08'),
  ('NSE_EQ|INE437A01024', 'APOLLOHOSP', 'Apollo Hospitals Enterprise Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Pharma'), 1, 0.05, TRUE, '1996-01-10'),
  ('NSE_EQ|INE361B01024', 'DIVISLAB', 'Divi''s Laboratories Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Pharma'), 1, 0.05, TRUE, '2003-03-12'),

  -- Auto
  ('NSE_EQ|INE155A01022', 'TATAMOTORS', 'Tata Motors Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Auto'), 1, 0.05, TRUE, '1998-07-22'),
  ('NSE_EQ|INE585B01010', 'MARUTI', 'Maruti Suzuki India Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Auto'), 1, 0.05, TRUE, '2003-07-09'),
  ('NSE_EQ|INE101A01026', 'M&M', 'Mahindra & Mahindra Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Auto'), 1, 0.05, TRUE, '1996-01-03'),
  ('NSE_EQ|INE917I01010', 'BAJAJ-AUTO', 'Bajaj Auto Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Auto'), 1, 0.05, TRUE, '2008-05-26'),
  ('NSE_EQ|INE066A01021', 'EICHERMOT', 'Eicher Motors Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Auto'), 1, 0.05, TRUE, '2004-09-08'),

  -- FMCG
  ('NSE_EQ|INE030A01027', 'HINDUNILVR', 'Hindustan Unilever Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'FMCG'), 1, 0.05, TRUE, '1995-07-06'),
  ('NSE_EQ|INE154A01025', 'ITC', 'ITC Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'FMCG'), 1, 0.05, TRUE, '1995-05-03'),
  ('NSE_EQ|INE280A01028', 'TITAN', 'Titan Company Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'FMCG'), 1, 0.05, TRUE, '1995-10-18'),
  ('NSE_EQ|INE239A01016', 'NESTLEIND', 'Nestle India Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'FMCG'), 1, 0.05, TRUE, '2008-06-25'),
  ('NSE_EQ|INE216A01030', 'BRITANNIA', 'Britannia Industries Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'FMCG'), 1, 0.05, TRUE, '1998-11-05'),
  ('NSE_EQ|INE021A01026', 'ASIANPAINT', 'Asian Paints Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'FMCG'), 1, 0.05, TRUE, '1995-05-31'),
  ('NSE_EQ|INE192A01025', 'TATACONSUM', 'Tata Consumer Products Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'FMCG'), 1, 0.05, TRUE, '1998-11-25'),

  -- Energy
  ('NSE_EQ|INE002A01018', 'RELIANCE', 'Reliance Industries Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Energy'), 1, 0.05, TRUE, '1995-11-29'),
  ('NSE_EQ|INE733E01010', 'NTPC', 'NTPC Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Energy'), 1, 0.05, TRUE, '2004-11-05'),
  ('NSE_EQ|INE213A01029', 'ONGC', 'Oil and Natural Gas Corporation Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Energy'), 1, 0.05, TRUE, '1995-07-19'),
  ('NSE_EQ|INE752E01010', 'POWERGRID', 'Power Grid Corporation of India Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Energy'), 1, 0.05, TRUE, '2007-10-05'),
  ('NSE_EQ|INE522F01014', 'COALINDIA', 'Coal India Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Energy'), 1, 0.05, TRUE, '2010-11-04'),

  -- Metals
  ('NSE_EQ|INE081A01020', 'TATASTEEL', 'Tata Steel Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Metals'), 1, 0.01, TRUE, '1998-11-18'),
  ('NSE_EQ|INE019A01038', 'JSWSTEEL', 'JSW Steel Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Metals'), 1, 0.05, TRUE, '2005-03-24'),
  ('NSE_EQ|INE038A01020', 'HINDALCO', 'Hindalco Industries Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Metals'), 1, 0.05, TRUE, '1996-05-15'),

  -- Telecom
  ('NSE_EQ|INE397D01024', 'BHARTIARTL', 'Bharti Airtel Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Telecom'), 1, 0.05, TRUE, '2002-02-18'),
  ('NSE_EQ|INE669E01016', 'IDEA', 'Vodafone Idea Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Telecom'), 1, 0.01, TRUE, '2007-03-09'),

  -- Realty
  ('NSE_EQ|INE271C01023', 'DLF', 'DLF Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Realty'), 1, 0.05, TRUE, '2007-07-05'),
  ('NSE_EQ|INE484J01027', 'GODREJPROP', 'Godrej Properties Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Realty'), 1, 0.05, TRUE, '2010-01-05'),

  -- PSU Bank
  ('NSE_EQ|INE062A01020', 'SBIN', 'State Bank of India', 'NSE', (SELECT id FROM sectors WHERE name = 'PSU Bank'), 1, 0.05, TRUE, '1995-03-01'),
  ('NSE_EQ|INE077A01032', 'BANKBARODA', 'Bank of Baroda', 'NSE', (SELECT id FROM sectors WHERE name = 'PSU Bank'), 1, 0.05, TRUE, '1997-02-19'),
  ('NSE_EQ|INE160A01022', 'PNB', 'Punjab National Bank', 'NSE', (SELECT id FROM sectors WHERE name = 'PSU Bank'), 1, 0.01, TRUE, '2002-04-26'),
  ('NSE_EQ|INE476A01014', 'CANBK', 'Canara Bank', 'NSE', (SELECT id FROM sectors WHERE name = 'PSU Bank'), 1, 0.01, TRUE, '2002-12-23'),

  -- Media
  ('NSE_EQ|INE256A01028', 'ZEEL', 'Zee Entertainment Enterprises Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Media'), 1, 0.01, TRUE, '1993-11-09'),
  ('NSE_EQ|INE424H01027', 'SUNTV', 'Sun TV Network Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Media'), 1, 0.05, TRUE, '2006-04-24'),
  ('NSE_EQ|INE191H01014', 'PVRINOX', 'PVR INOX Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Media'), 1, 0.05, TRUE, '2006-01-18'),

  -- Infra
  ('NSE_EQ|INE018A01030', 'LT', 'Larsen & Toubro Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Infra'), 1, 0.05, TRUE, '2004-06-23'),
  ('NSE_EQ|INE423A01024', 'ADANIENT', 'Adani Enterprises Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Infra'), 1, 0.05, TRUE, '1997-06-04'),
  ('NSE_EQ|INE742F01042', 'ADANIPORTS', 'Adani Ports and Special Economic Zone Limited', 'NSE', (SELECT id FROM sectors WHERE name = 'Infra'), 1, 0.05, TRUE, '2007-11-27')
ON CONFLICT (instrument_key) DO NOTHING;
