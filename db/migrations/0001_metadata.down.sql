-- SS-002: Revert Core Metadata Schema
DROP TABLE IF EXISTS market_sessions CASCADE;
DROP TABLE IF EXISTS exchange_holidays CASCADE;
DROP INDEX IF EXISTS stocks_sector_idx;
DROP TABLE IF EXISTS stocks CASCADE;
DROP TABLE IF EXISTS sectors CASCADE;
