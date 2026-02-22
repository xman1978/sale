-- 对话式销售日志录入系统数据库架构

-- 创建schema，并设置为当前schema
CREATE SCHEMA IF NOT EXISTS sale AUTHORIZATION sale;
SET search_path TO sale;

-- 创建表
-- 用户表
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(255) PRIMARY KEY,  -- 飞书用户open_id
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_dialogs_session_id ON dialogs(session_id);
CREATE INDEX IF NOT EXISTS idx_dialogs_turn_index ON dialogs(session_id, turn_index);
CREATE INDEX IF NOT EXISTS idx_follow_records_user_id ON follow_records(user_id);
CREATE INDEX IF NOT EXISTS idx_follow_records_customer_id ON follow_records(customer_id);

-- demo_user（非飞书环境下使用）
INSERT INTO sale.users (id, name, orgname, status) VALUES ('demo_user', 'Demo User', '', 0) ON CONFLICT (id) DO NOTHING;

-- 兼容已有库：为 users 添加 avatar_url
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'sale' AND table_name = 'users') THEN
        IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'sale' AND table_name = 'users' AND column_name = 'avatar_url') THEN
            ALTER TABLE sale.users ADD COLUMN avatar_url VARCHAR(500);
        END IF;
    END IF;
END $$;

