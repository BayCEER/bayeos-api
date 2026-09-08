CREATE TABLE IF NOT EXISTS series (
  id SYMBOL INDEX ,
  ts TIMESTAMP,
  value DOUBLE,
  status byte
) TIMESTAMP(ts) PARTITION BY DAY WAL
DEDUP UPSERT KEYS(id, ts);

CREATE MATERIALIZED VIEW IF NOT EXISTS series_view_1h AS
SELECT
  id,
  ts as bucket_ts,      
  first(value) AS first,
  max(value) AS max,
  min(value) AS min,
  last(value) AS last,
  sum(value) AS sum,
  count(value) AS count,
  avg(value) AS avg
FROM series
SAMPLE BY 1h;

CREATE MATERIALIZED VIEW  IF NOT EXISTS series_view_1d AS
SELECT
  id,
  bucket_ts,      
  first(first) AS first,
  max(max) AS max,
  min(min) AS min,
  last(last) AS last,
  sum(sum) AS sum,
  count(count) AS count,
  avg(avg) AS avg
FROM series_view_1h
SAMPLE BY 1d;

CREATE MATERIALIZED VIEW IF NOT EXISTS series_view_1M AS
SELECT
  id,
  bucket_ts,      
  first(first) AS first,
  max(max) AS max,
  min(min) AS min,
  last(last) AS last,
  sum(sum) AS sum,
  count(count) AS count,
  avg(avg) AS avg
FROM series_view_1d
SAMPLE BY 1M;

CREATE MATERIALIZED VIEW IF NOT EXISTS series_view_1y AS
SELECT
  id,
  bucket_ts,      
  first(first) AS first,
  max(max) AS max,
  min(min) AS min,
  last(last) AS last,
  sum(sum) AS sum,
  count(count) AS count,
  avg(avg) AS avg
FROM series_view_1M
SAMPLE BY 1y;





