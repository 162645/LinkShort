-- ClickHouse Analytics Service - Step 2: Create Tables
-- This script assumes the database 'analytics' exists and connection is made to it

-- Click analytics table optimized for time-series queries
CREATE TABLE IF NOT EXISTS click_analytics (
    short_code String,
    long_url String,
    client_ip String,
    user_agent String,
    referrer String,
    country String,
    city String,
    device_type String,
    browser String,
    os String,
    timestamp DateTime64(3) DEFAULT now(),
    session_id String,
    is_unique UInt8 DEFAULT 0,
    created_at DateTime64(3) DEFAULT now()
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (short_code, timestamp)
SETTINGS index_granularity = 8192;

-- URL summary table for fast aggregations
CREATE TABLE IF NOT EXISTS url_summary (
    short_code String,
    date Date,
    hour UInt8,
    total_clicks UInt64,
    unique_clicks UInt64,
    top_country String,
    top_device String,
    top_browser String,
    created_at DateTime64(3) DEFAULT now(),
    updated_at DateTime64(3) DEFAULT now()
) ENGINE = ReplacingMergeTree(updated_at)
PARTITION BY toYYYYMM(date)
ORDER BY (short_code, date, hour)
SETTINGS index_granularity = 8192;
