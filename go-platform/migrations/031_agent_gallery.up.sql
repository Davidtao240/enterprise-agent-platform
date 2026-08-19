-- M7-B: Agent Gallery — agent_packages (discovery/selection layer) + usage stats

CREATE TABLE agent_packages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    package_code VARCHAR(64) NOT NULL,
    name VARCHAR(128) NOT NULL,
    description TEXT,
    category VARCHAR(32) NOT NULL
        CHECK (category IN ('general', 'departmental')),
    business_app_code VARCHAR(64) NOT NULL,
    graph_key VARCHAR(128) NOT NULL,
    graph_version VARCHAR(32) DEFAULT 'v1',
    entry_type VARCHAR(32) NOT NULL DEFAULT 'conversation'
        CHECK (entry_type IN ('conversation', 'form')),
    icon VARCHAR(64),
    capabilities_json JSONB DEFAULT '{}',
    sample_prompts_json JSONB DEFAULT '[]',
    status VARCHAR(32) NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'published', 'disabled')),
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    UNIQUE (tenant_id, package_code)
);

CREATE INDEX idx_agent_packages_tenant_status ON agent_packages(tenant_id, status);
CREATE INDEX idx_agent_packages_category ON agent_packages(tenant_id, category, status);
CREATE INDEX idx_agent_packages_business_app ON agent_packages(tenant_id, business_app_code, status);

-- Read-only usage stats table (written by Asynq daily aggregation task)
CREATE TABLE agent_package_usage_stats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    package_code VARCHAR(64) NOT NULL,
    stat_date DATE NOT NULL,
    conversation_count INTEGER NOT NULL DEFAULT 0,
    message_count INTEGER NOT NULL DEFAULT 0,
    token_count INTEGER NOT NULL DEFAULT 0,
    avg_duration_seconds NUMERIC(10,2) DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, package_code, stat_date)
);

CREATE INDEX idx_usage_stats_tenant_package ON agent_package_usage_stats(tenant_id, package_code, stat_date DESC);

-- Seed: 3 agent packages for M7 demonstration
INSERT INTO agent_packages (tenant_id, package_code, name, description, category, business_app_code, graph_key, entry_type, icon, capabilities_json, sample_prompts_json, status, published_at)
SELECT
    t.id,
    'finance_operating_report_assistant',
    '财务经营报告助手',
    '一句话生成部门经营报告：自动完成数据校验、波动分析与归因初判，超阈值结论自动走审批。',
    'departmental',
    'finance',
    'finance_chat_graph',
    'conversation',
    'finance-chart',
    '{"domain": "finance", "features": ["经营报告", "波动分析", "归因初判", "自动审批"], "languages": ["zh-CN"]}'::jsonb,
    '["帮我生成 2026 Q3 华东销售部的经营报告，重点看费用异常", "对比 2026 Q2 与 Q3 的毛利率变化，找出波动最大的三个科目", "把上次报告的口径换成事业部维度，重新出一版"]'::jsonb,
    'published',
    NOW()
FROM tenants t WHERE t.code = 'default'
ON CONFLICT DO NOTHING;

INSERT INTO agent_packages (tenant_id, package_code, name, description, category, business_app_code, graph_key, entry_type, icon, capabilities_json, sample_prompts_json, status, published_at)
SELECT
    t.id,
    'document_summary_assistant',
    '文档总结助手',
    '上传长文档秒级提炼：摘要、核心结论与行动项，可继续追问细节。',
    'general',
    'productivity',
    'document_summary_graph',
    'conversation',
    'document-text',
    '{"domain": "productivity", "features": ["摘要生成", "关键结论", "行动项提取", "多轮追问"], "languages": ["zh-CN", "en-US"]}'::jsonb,
    '["帮我总结这份季度战略文档的核心要点", "从这份技术方案中列出所有待决策项和风险", "对比这两份合同的关键条款差异"]'::jsonb,
    'published',
    NOW()
FROM tenants t WHERE t.code = 'default'
ON CONFLICT DO NOTHING;

INSERT INTO agent_packages (tenant_id, package_code, name, description, category, business_app_code, graph_key, entry_type, icon, capabilities_json, sample_prompts_json, status, published_at)
SELECT
    t.id,
    'meeting_minutes_assistant',
    '会议纪要助手',
    '粘贴转录或上传会议稿，自动生成结构化纪要：决议、行动项、负责人与截止时间。',
    'general',
    'productivity',
    'meeting_minutes_graph',
    'conversation',
    'calendar-note',
    '{"domain": "productivity", "features": ["纪要生成", "行动项提取", "负责人识别", "截止时间提取"], "languages": ["zh-CN"]}'::jsonb,
    '["根据这段产品评审录音，生成结构化会议纪要", "从会议转录中提取所有行动项和负责人", "对比两次项目周会的决议跟进情况"]'::jsonb,
    'published',
    NOW()
FROM tenants t WHERE t.code = 'default'
ON CONFLICT DO NOTHING;