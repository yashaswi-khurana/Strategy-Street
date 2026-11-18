-- SS-003: Revert Candle Schema and Hypertables
DROP TABLE IF EXISTS candles_1m CASCADE;
DROP TABLE IF EXISTS candles_1s CASCADE;
