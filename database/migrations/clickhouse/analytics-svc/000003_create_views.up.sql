-- ClickHouse Analytics Service - Step 3: Create Views
-- Daily aggregation materialized view for dashboard queries
CREATE MATERIALIZED VIEW IF NOT EXISTS daily_analytics
TO url_summary
AS SELECT
    short_code,
    toDate(timestamp) as date,
    toHour(timestamp) as hour,
    count() as total_clicks,
    uniq(session_id) as unique_clicks,
    topK(1)(country)[1] as top_country,
    topK(1)(device_type)[1] as top_device,
    topK(1)(browser)[1] as top_browser,
    now() as created_at,
    now() as updated_at
FROM click_analytics
GROUP BY short_code, date, hour;

-- Real-time dashboard view for fast queries
CREATE VIEW IF NOT EXISTS dashboard_metrics AS
SELECT 
    toInt64(uniq(short_code)) as total_urls,
    toInt64(count()) as total_clicks,
    toInt64(uniq(session_id)) as unique_clicks,
    toInt64(uniqExact(short_code)) as active_urls
FROM click_analytics 
WHERE timestamp >= now() - INTERVAL 30 DAY;

-- Top URLs view for analytics
CREATE VIEW IF NOT EXISTS top_urls AS
SELECT 
    short_code,
    count() as total_clicks,
    uniq(session_id) as unique_clicks,
    max(timestamp) as last_click
FROM click_analytics 
WHERE timestamp >= now() - INTERVAL 7 DAY
GROUP BY short_code
ORDER BY total_clicks DESC;

-- Country analytics view
CREATE VIEW IF NOT EXISTS country_analytics AS
SELECT 
    short_code,
    country,
    count() as clicks,
    uniq(session_id) as unique_visitors
FROM click_analytics 
WHERE timestamp >= now() - INTERVAL 7 DAY
  AND country != ''
GROUP BY short_code, country
ORDER BY short_code, clicks DESC;
