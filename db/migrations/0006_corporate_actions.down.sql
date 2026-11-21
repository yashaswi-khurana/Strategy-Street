-- Rollback SS-006: Drop adjusted_candles function and corporate action tables

DROP FUNCTION IF EXISTS adjusted_candles(INT, TEXT, TIMESTAMPTZ, TIMESTAMPTZ);

DROP TABLE IF EXISTS data_quality_issues CASCADE;
DROP TABLE IF EXISTS adjustment_factors CASCADE;
DROP TABLE IF EXISTS corporate_actions CASCADE;

