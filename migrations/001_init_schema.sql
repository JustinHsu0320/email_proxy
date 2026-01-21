-- migrations/001_init_schema.sql
-- SMTP 系統資料庫初始化

-- ============================================
-- 郵件表
-- ============================================
CREATE TABLE IF NOT EXISTS mails (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_address VARCHAR(255) NOT NULL,
    to_addresses TEXT[] NOT NULL,
    cc_addresses TEXT[],
    bcc_addresses TEXT[],
    subject TEXT NOT NULL,
    body TEXT,
    html TEXT,
    status VARCHAR(50) NOT NULL DEFAULT 'queued',
    retry_count INT DEFAULT 0,
    error_message TEXT,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    client_id VARCHAR(100) NOT NULL,
    client_name VARCHAR(255),
    metadata JSONB
);

-- ============================================
-- Client Token 表
-- ============================================
CREATE TABLE IF NOT EXISTS client_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id VARCHAR(100) UNIQUE NOT NULL,
    client_name VARCHAR(255) NOT NULL,
    department VARCHAR(100),
    permissions TEXT[] NOT NULL,
    token_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    revoked_at TIMESTAMPTZ,
    is_active BOOLEAN DEFAULT TRUE
);

-- ============================================
-- API 請求日誌表
-- ============================================
CREATE TABLE IF NOT EXISTS api_logs (
    id BIGSERIAL PRIMARY KEY,
    client_id VARCHAR(100) NOT NULL,
    client_name VARCHAR(255),
    request_ip INET,
    endpoint VARCHAR(255) NOT NULL,
    method VARCHAR(10) NOT NULL,
    mail_id UUID REFERENCES mails(id),
    status_code INT,
    response_time_ms INT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================
-- 附件表
-- ============================================
CREATE TABLE IF NOT EXISTS attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mail_id UUID REFERENCES mails(id) ON DELETE CASCADE,
    filename VARCHAR(255) NOT NULL,
    content_type VARCHAR(100),
    size_bytes BIGINT,
    storage_path VARCHAR(500) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================
-- 索引
-- ============================================
CREATE INDEX IF NOT EXISTS idx_mails_status ON mails(status);
CREATE INDEX IF NOT EXISTS idx_mails_client_id ON mails(client_id);
CREATE INDEX IF NOT EXISTS idx_mails_created_at ON mails(created_at);
CREATE INDEX IF NOT EXISTS idx_api_logs_client_id ON api_logs(client_id);
CREATE INDEX IF NOT EXISTS idx_api_logs_created_at ON api_logs(created_at);
