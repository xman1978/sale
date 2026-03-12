-- 对话式销售日志录入系统数据库架构

-- 创建schema，并设置为当前schema
CREATE SCHEMA IF NOT EXISTS sale AUTHORIZATION sale;
SET search_path TO sale;

-- 创建表
-- 用户表
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(255) PRIMARY KEY,  -- 飞书用户 union_id
    name VARCHAR(255) NOT NULL,
    phone VARCHAR(255),
    status INTEGER NOT NULL DEFAULT 0, -- 0 在职/1 离职
    orgname VARCHAR(2000) NOT NULL, -- 组织名称
    avatar_url VARCHAR(500),       -- 头像（OAuth 获取）
    start_lark TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 客户表
CREATE TABLE IF NOT EXISTS customers (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    contact_person VARCHAR(255),
    contact_phone VARCHAR(255),
    contact_role VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 会话表
CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL REFERENCES users(id),
    status VARCHAR(255) NOT NULL, -- COLLECTING/CONFIRMING/OUTPUTTING/EXIT
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at TIMESTAMPTZ
);

-- 对话表
CREATE TABLE IF NOT EXISTS dialogs (
    id UUID PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES sessions(id),
    state VARCHAR(255) NOT NULL, -- CUSTOMER_NAME/FOLLOW_METHOD/FOLLOW_CONTENT/FOLLOW_GOAL/FOLLOW_RESULT/NEXT_PLAN/COMPLETE
    status VARCHAR(255) NOT NULL, -- COLLECTING/CONFIRMING/OUTPUTTING/EXIT
    turn_index INTEGER NOT NULL,
    focus_customer_id UUID REFERENCES customers(id),
    is_first_focus BOOLEAN NOT NULL DEFAULT false,
    semantic_relevance VARCHAR(50),
    pending_updates JSONB,
    runtime_snapshot JSONB NOT NULL,
    turn_content TEXT,    -- 本轮对话内容，格式：User: …\nAssistant: …，供大模型理解上下文
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(session_id, turn_index)
);

-- 跟进记录表
CREATE TABLE IF NOT EXISTS follow_records (
    id UUID PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL REFERENCES users(id),
    customer_id UUID NOT NULL REFERENCES customers(id),
    customer_name VARCHAR(255) NOT NULL,
    contact_person VARCHAR(255),
    contact_phone VARCHAR(255),
    contact_role VARCHAR(255),
    follow_time TIMESTAMPTZ NOT NULL,
    follow_method VARCHAR(255),
    follow_content VARCHAR(2000),
    follow_goal VARCHAR(2000),
    follow_result VARCHAR(2000),
    risk_content VARCHAR(2000),
    next_plan VARCHAR(2000),
    ai BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_dialogs_session_id ON dialogs(session_id);
CREATE INDEX IF NOT EXISTS idx_dialogs_turn_index ON dialogs(session_id, turn_index);
CREATE INDEX IF NOT EXISTS idx_follow_records_user_id ON follow_records(user_id,created_at);
CREATE INDEX IF NOT EXISTS idx_follow_records_customer_id ON follow_records(customer_id);

-- 管理员可查看的日志范围：manager_id 为可查看列表的用户，每管理员一行
-- user_id 存储以逗号分隔的被查看用户 id；若为 '0' 表示可看所有用户
CREATE TABLE IF NOT EXISTS records_scope (
    manager_id VARCHAR(255) PRIMARY KEY REFERENCES users(id),
    user_id    VARCHAR(2000) NOT NULL
);

-- 热词提取：关键词明细表（每条跟进记录抽取出的关键词）
CREATE TABLE IF NOT EXISTS sales_keyword_records (
    id          UUID PRIMARY KEY,
    category    VARCHAR(64) NOT NULL,
    term        VARCHAR(255) NOT NULL,
    count       INT NOT NULL,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_sales_keyword_records_create_time ON sales_keyword_records(create_time);
CREATE INDEX IF NOT EXISTS idx_sales_keyword_records_category_term ON sales_keyword_records(category, term);

-- 热词提取：同义词词典（source_term -> target_term，抽取后归一用）
CREATE TABLE IF NOT EXISTS sales_keyword_synonyms (
    source_term VARCHAR(255) NOT NULL,
    target_term VARCHAR(255) NOT NULL,
    PRIMARY KEY (source_term)
);

-- 热词提取：统计结果表（每次统计任务写入一批）
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

