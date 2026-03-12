SET search_path TO sale;

-- 热词提取相关表（在 sale schema 下执行）
CREATE TABLE IF NOT EXISTS sales_keyword_records (
    id          UUID PRIMARY KEY,
    category    VARCHAR(64) NOT NULL,
    term        VARCHAR(255) NOT NULL,
    count       INT NOT NULL,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_sales_keyword_records_create_time ON sales_keyword_records(create_time);
CREATE INDEX IF NOT EXISTS idx_sales_keyword_records_category_term ON sales_keyword_records(category, term);

CREATE TABLE IF NOT EXISTS sales_keyword_synonyms (
    source_term VARCHAR(255) NOT NULL,
    target_term VARCHAR(255) NOT NULL,
    PRIMARY KEY (source_term)
);

CREATE TABLE IF NOT EXISTS sales_hot_words_stats (
    id               UUID PRIMARY KEY,
    run_time         TIMESTAMPTZ NOT NULL,
    time_window_days INT NOT NULL,
    category         VARCHAR(64) NOT NULL,
    term             VARCHAR(255) NOT NULL,
    frequency        INT NOT NULL,
    rank             INT NOT NULL,
    create_time      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (run_time, time_window_days, category, term)
);
CREATE INDEX IF NOT EXISTS idx_sales_hot_words_stats_run_window ON sales_hot_words_stats(run_time, time_window_days);
CREATE INDEX IF NOT EXISTS idx_sales_hot_words_stats_category_run ON sales_hot_words_stats(category, run_time);
