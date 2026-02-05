-- migrations/002_email_sender_configs.sql
-- Email Sender Config 表 - 儲存每個 Client 的 Microsoft OAuth 配置

-- ============================================
-- Email Sender Configs 表
-- ============================================
CREATE TABLE IF NOT EXISTS email_sender_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_token_id UUID NOT NULL REFERENCES client_tokens(id) ON DELETE CASCADE,
    sender_email VARCHAR(255) NOT NULL,
    ms_tenant_id VARCHAR(255) NOT NULL,
    ms_client_id VARCHAR(255) NOT NULL,
    ms_client_secret_encrypted VARCHAR(1000) NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(client_token_id, sender_email)
);

-- ============================================
-- 索引
-- ============================================
CREATE INDEX IF NOT EXISTS idx_email_sender_configs_client_token_id 
    ON email_sender_configs(client_token_id);
CREATE INDEX IF NOT EXISTS idx_email_sender_configs_sender_email 
    ON email_sender_configs(sender_email);

-- ============================================
-- 更新 mails 表 - 新增 sender_config_id 欄位
-- ============================================
ALTER TABLE mails ADD COLUMN IF NOT EXISTS sender_config_id UUID REFERENCES email_sender_configs(id);

CREATE INDEX IF NOT EXISTS idx_mails_sender_config_id ON mails(sender_config_id);
