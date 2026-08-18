import { useCallback, useEffect, useState } from 'react';
import { Card, Space, Table, Tag, Typography } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { Button } from 'antd';
import { getConnectorRegistry, getConnectorBindings, getDomainPolicies } from '../services/api';
import PayloadViewer from '../components/PayloadViewer';
import { useAuthStore } from '../store/auth';

const { Title } = Typography;

// ── 模型(M3-A / policy 契约) ──

interface RegistryEntry {
  id: string;
  connector_code: string;
  version: string;
  connector_type: string;
  capabilities_json: string;
  auth_type: string;
  release_stage: string;
  status: string;
  updated_at: string;
}

interface ConnectorBinding {
  id: string;
  business_app_code: string;
  connector_code: string;
  name: string;
  status: string;
  environment: string;
  allowed_capabilities: string;
  connector_version?: string;
  updated_at: string;
}

interface DomainPolicy {
  id: string;
  business_app_code: string;
  allowed_agent_domains_json: string;
  allowed_tool_domains_json: string;
  allow_shared_agents: boolean;
  allow_shared_tools: boolean;
  high_risk_requires_review: boolean;
  status: string;
}

const ENV_COLOR: Record<string, string> = {
  mock: 'default',
  sandbox: 'blue',
  shadow: 'purple',
  production: 'red',
};

export default function ConnectorScopePage() {
  const hasPermission = useAuthStore((s) => s.hasPermission);
  const canReadPolicies = hasPermission('business_app:read');

  const [registry, setRegistry] = useState<RegistryEntry[]>([]);
  const [bindings, setBindings] = useState<ConnectorBinding[]>([]);
  const [policies, setPolicies] = useState<DomainPolicy[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [reg, bind] = await Promise.all([getConnectorRegistry(), getConnectorBindings()]);
      setRegistry(reg.data.data.items || []);
      setBindings(bind.data.data.items || []);
      if (canReadPolicies) {
        const pol = await getDomainPolicies();
        setPolicies(pol.data.data || []);
      }
    } finally {
      setLoading(false);
    }
  }, [canReadPolicies]);

  useEffect(() => {
    load();
  }, [load]);

  const registryColumns = [
    { title: '连接器', dataIndex: 'connector_code', width: 140 },
    { title: '版本', dataIndex: 'version', width: 90 },
    { title: '类型', dataIndex: 'connector_type', width: 120 },
    { title: '认证方式', dataIndex: 'auth_type', width: 110 },
    { title: '发布阶段', dataIndex: 'release_stage', width: 110, render: (v: string) => <Tag color={v === 'production' ? 'red' : v === 'shadow' ? 'purple' : 'blue'}>{v}</Tag> },
    { title: '状态', dataIndex: 'status', width: 100, render: (v: string) => <Tag color={v === 'active' ? 'success' : v === 'deprecated' ? 'error' : 'default'}>{v}</Tag> },
    { title: '能力声明', dataIndex: 'capabilities_json', render: (v: string) => <PayloadViewer value={v} /> },
  ];

  const bindingColumns = [
    { title: '名称', dataIndex: 'name', width: 150 },
    { title: '业务域', dataIndex: 'business_app_code', width: 100 },
    { title: '连接器', dataIndex: 'connector_code', width: 130 },
    { title: '环境', dataIndex: 'environment', width: 110, render: (v: string) => <Tag color={ENV_COLOR[v] || 'default'}>{v}</Tag> },
    { title: '允许能力', dataIndex: 'allowed_capabilities', width: 170, render: (v: string) => <PayloadViewer value={v} /> },
    { title: '固定版本', dataIndex: 'connector_version', width: 100, render: (v?: string) => v || '最新 active' },
    { title: '状态', dataIndex: 'status', width: 100, render: (v: string) => <Tag color={v === 'active' ? 'success' : 'default'}>{v}</Tag> },
  ];

  const policyColumns = [
    { title: '业务域', dataIndex: 'business_app_code', width: 120 },
    { title: '允许 Agent 域', dataIndex: 'allowed_agent_domains_json', render: (v: string) => <PayloadViewer value={v} /> },
    { title: '允许 Tool 域', dataIndex: 'allowed_tool_domains_json', render: (v: string) => <PayloadViewer value={v} /> },
    { title: '共享 Agent', dataIndex: 'allow_shared_agents', width: 100, render: (v: boolean) => (v ? <Tag color="orange">允许</Tag> : <Tag>禁止</Tag>) },
    { title: '高风险需审批', dataIndex: 'high_risk_requires_review', width: 120, render: (v: boolean) => (v ? <Tag color="red">必须</Tag> : <Tag>否</Tag>) },
  ];

  return (
    <div>
      <Title level={4}>连接器管理 · 授权范围</Title>
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        <Card
          size="small"
          title="Connector Registry(注册表)"
          extra={<Button size="small" icon={<ReloadOutlined />} onClick={load}>刷新</Button>}
        >
          <Table
            rowKey="id"
            size="small"
            loading={loading}
            dataSource={registry}
            columns={registryColumns}
            pagination={{ pageSize: 10, showSizeChanger: false }}
            scroll={{ x: 1000 }}
          />
        </Card>

        <Card size="small" title="Connector Bindings(工具 ↔ 连接器授权关系,租户内)">
          <Table
            rowKey="id"
            size="small"
            loading={loading}
            dataSource={bindings}
            columns={bindingColumns}
            pagination={{ pageSize: 10, showSizeChanger: false }}
            scroll={{ x: 1000 }}
          />
        </Card>

        {canReadPolicies && (
          <Card size="small" title="域策略(受控路由的域约束)">
            <Table
              rowKey="id"
              size="small"
              dataSource={policies}
              columns={policyColumns}
              pagination={false}
              scroll={{ x: 900 }}
            />
          </Card>
        )}
      </Space>
    </div>
  );
}
