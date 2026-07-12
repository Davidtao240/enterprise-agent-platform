import { useEffect, useState } from 'react';
import { Alert, Button, Descriptions, Drawer, Form, Input, Select, Space, Table, Tabs, Tag, Typography } from 'antd';
import { getAgents, getBusinessAppRegistry, getDomainPolicies, getTools, listWorkflowTemplates } from '../services/api';
import { useAuthStore } from '../store/auth';
import { tDomain, tRisk, tStatus } from '../utils/i18n';

const { Title } = Typography;

function getValue(record: any, snakeKey: string, camelKey: string) {
  return record?.[snakeKey] ?? record?.[camelKey];
}

function prettyJSON(raw: unknown) {
  if (!raw) return '{}';
  if (typeof raw !== 'string') return JSON.stringify(raw, null, 2);
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}

const detailFieldLabels: Record<string, string> = {
  capabilities_json: '能力定义',
  definition_json: '模板定义',
  input_schema_json: '输入结构',
  output_schema_json: '输出结构',
};

export default function RegistryPage() {
  const hasPermission = useAuthStore((s) => s.hasPermission);
  const canReadBusinessApps = hasPermission('business_app:read');
  const canReadTemplates = hasPermission('workflow_template:read');
  const canManageAgents = hasPermission('agent:manage');
  const canManageTools = hasPermission('tool:manage');
  const [businessApps, setBusinessApps] = useState<any[]>([]);
  const [domainPolicies, setDomainPolicies] = useState<any[]>([]);
  const [templates, setTemplates] = useState<any[]>([]);
  const [agents, setAgents] = useState<any[]>([]);
  const [tools, setTools] = useState<any[]>([]);
  const [businessAppLoading, setBusinessAppLoading] = useState(false);
  const [policyLoading, setPolicyLoading] = useState(false);
  const [templateLoading, setTemplateLoading] = useState(false);
  const [agentLoading, setAgentLoading] = useState(false);
  const [toolLoading, setToolLoading] = useState(false);
  const [selected, setSelected] = useState<{ title: string; record: any; fields: string[] } | null>(null);
  const noRegistryPermission = !canReadBusinessApps && !canReadTemplates && !canManageAgents && !canManageTools;

  const loadBusinessApps = (params: Record<string, string> = {}) => {
    if (!canReadBusinessApps) return;
    setBusinessAppLoading(true);
    getBusinessAppRegistry(params)
      .then(({ data }) => setBusinessApps(data.data || []))
      .finally(() => setBusinessAppLoading(false));
  };

  const loadDomainPolicies = (params: Record<string, string> = {}) => {
    if (!canReadBusinessApps) return;
    setPolicyLoading(true);
    getDomainPolicies(params)
      .then(({ data }) => setDomainPolicies(data.data || []))
      .finally(() => setPolicyLoading(false));
  };

  const loadTemplates = (params: Record<string, string> = {}) => {
    if (!canReadTemplates) return;
    setTemplateLoading(true);
    listWorkflowTemplates(params)
      .then(({ data }) => setTemplates(data.data || []))
      .finally(() => setTemplateLoading(false));
  };

  const loadAgents = (params: Record<string, string> = {}) => {
    if (!canManageAgents) return;
    setAgentLoading(true);
    getAgents(params)
      .then(({ data }) => setAgents(data.data || []))
      .finally(() => setAgentLoading(false));
  };

  const loadTools = (params: Record<string, string> = {}) => {
    if (!canManageTools) return;
    setToolLoading(true);
    getTools(params)
      .then(({ data }) => setTools(data.data || []))
      .finally(() => setToolLoading(false));
  };

  useEffect(() => {
    loadBusinessApps();
    loadDomainPolicies();
    loadTemplates();
    loadAgents();
    loadTools();
  }, [canReadBusinessApps, canReadTemplates, canManageAgents, canManageTools]);

  const businessAppColumns = [
    { title: '业务应用', dataIndex: 'code', key: 'code', render: (v: string) => tDomain(v) },
    { title: '编码', dataIndex: 'code', key: 'code_raw' },
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '描述', dataIndex: 'description', key: 'description', ellipsis: true, render: (v: string) => v || '-' },
    { title: '排序', dataIndex: 'sort_order', key: 'sort_order', width: 90 },
    { title: '状态', dataIndex: 'status', key: 'status', render: (status: string) => <Tag>{tStatus(status)}</Tag> },
  ];

  const policyColumns = [
    { title: '业务应用', dataIndex: 'business_app_code', key: 'business_app_code', render: (v: string) => tDomain(v) },
    { title: 'Agent 领域', dataIndex: 'allowed_agent_domains_json', key: 'allowed_agent_domains_json', render: (v: string) => prettyJSON(v) },
    { title: 'Tool 领域', dataIndex: 'allowed_tool_domains_json', key: 'allowed_tool_domains_json', render: (v: string) => prettyJSON(v) },
    { title: '共享 Agent', dataIndex: 'allow_shared_agents', key: 'allow_shared_agents', render: (v: boolean) => v ? <Tag color="blue">允许</Tag> : <Tag>禁止</Tag> },
    { title: '共享 Tool', dataIndex: 'allow_shared_tools', key: 'allow_shared_tools', render: (v: boolean) => v ? <Tag color="blue">允许</Tag> : <Tag>禁止</Tag> },
    { title: '高风险复核', dataIndex: 'high_risk_requires_review', key: 'high_risk_requires_review', render: (v: boolean) => v ? <Tag color="warning">需要</Tag> : <Tag>不需要</Tag> },
    { title: '状态', dataIndex: 'status', key: 'status', render: (status: string) => <Tag>{tStatus(status)}</Tag> },
  ];

  const templateColumns = [
    { title: '业务应用', dataIndex: 'business_app_code', key: 'business_app_code', render: (v: string) => tDomain(v) },
    { title: '模板标识', dataIndex: 'workflow_template_key', key: 'workflow_template_key' },
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '版本', dataIndex: 'version', key: 'version' },
    { title: '图标识', dataIndex: 'graph_key', key: 'graph_key' },
    { title: '状态', dataIndex: 'status', key: 'status', render: (status: string) => <Tag>{tStatus(status)}</Tag> },
    {
      title: '操作',
      key: 'detail',
      render: (_: any, record: any) => (
        <Button size="small" onClick={() => setSelected({ title: '工作流模板详情', record, fields: ['definition_json'] })}>详情</Button>
      ),
    },
  ];

  const agentColumns = [
    { title: '智能体标识', dataIndex: 'agent_id', key: 'agent_id' },
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '领域', dataIndex: 'domain', key: 'domain', render: (v: string) => tDomain(v) },
    { title: '复用范围', dataIndex: 'reusable_scope', key: 'reusable_scope' },
    { title: '调用地址', dataIndex: 'endpoint', key: 'endpoint', render: (v: string) => v || '-' },
    { title: '状态', dataIndex: 'status', key: 'status', render: (status: string) => <Tag>{tStatus(status)}</Tag> },
    {
      title: '操作',
      key: 'detail',
      render: (_: any, record: any) => (
        <Button size="small" onClick={() => setSelected({ title: '智能体详情', record, fields: ['capabilities_json', 'input_schema_json', 'output_schema_json'] })}>详情</Button>
      ),
    },
  ];

  const toolColumns = [
    { title: '工具标识', key: 'tool_id', render: (_: any, record: any) => getValue(record, 'tool_id', 'ToolID') },
    { title: '名称', key: 'name', render: (_: any, record: any) => getValue(record, 'name', 'Name') },
    { title: '领域', key: 'domain', render: (_: any, record: any) => tDomain(getValue(record, 'domain', 'Domain')) },
    { title: '风险', key: 'risk', render: (_: any, record: any) => <Tag>{tRisk(getValue(record, 'risk_level', 'RiskLevel'))}</Tag> },
    { title: '共享', key: 'shared', render: (_: any, record: any) => getValue(record, 'is_shared', 'IsShared') ? <Tag color="blue">共享</Tag> : <Tag>领域内</Tag> },
    { title: '状态', key: 'status', render: (_: any, record: any) => <Tag>{tStatus(getValue(record, 'status', 'Status'))}</Tag> },
    {
      title: '操作',
      key: 'detail',
      render: (_: any, record: any) => (
        <Button size="small" onClick={() => setSelected({ title: '工具详情', record, fields: ['input_schema_json', 'output_schema_json'] })}>详情</Button>
      ),
    },
  ];

  return (
    <div>
      <Title level={4}>注册中心</Title>
      {noRegistryPermission && (
        <Alert type="info" showIcon message="当前账号没有可查看的注册中心权限" style={{ marginBottom: 16 }} />
      )}
      <Tabs
        items={[
          ...(canReadBusinessApps ? [{
            key: 'business-apps',
            label: '业务应用',
            children: (
              <Space direction="vertical" style={{ width: '100%' }} size="middle">
                <Form layout="inline" onFinish={(values) => loadBusinessApps(Object.fromEntries(Object.entries(values).filter(([, value]) => value)) as Record<string, string>)}>
                  <Form.Item name="status"><Input placeholder="状态" allowClear /></Form.Item>
                  <Form.Item><Button type="primary" htmlType="submit">搜索</Button></Form.Item>
                  <Form.Item><Button htmlType="reset" onClick={() => loadBusinessApps()}>重置</Button></Form.Item>
                </Form>
                <Table size="small" dataSource={businessApps} columns={businessAppColumns} rowKey="id" loading={businessAppLoading} scroll={{ x: 1000 }} locale={{ emptyText: '暂无业务应用记录' }} />
              </Space>
            ),
          }, {
            key: 'domain-policies',
            label: '域策略',
            children: (
              <Space direction="vertical" style={{ width: '100%' }} size="middle">
                <Form layout="inline" onFinish={(values) => loadDomainPolicies(Object.fromEntries(Object.entries(values).filter(([, value]) => value)) as Record<string, string>)}>
                  <Form.Item name="business_app_code"><Input placeholder="业务应用" allowClear /></Form.Item>
                  <Form.Item name="status"><Input placeholder="状态" allowClear /></Form.Item>
                  <Form.Item><Button type="primary" htmlType="submit">搜索</Button></Form.Item>
                  <Form.Item><Button htmlType="reset" onClick={() => loadDomainPolicies()}>重置</Button></Form.Item>
                </Form>
                <Table size="small" dataSource={domainPolicies} columns={policyColumns} rowKey="id" loading={policyLoading} scroll={{ x: 1200 }} locale={{ emptyText: '暂无域策略记录' }} />
              </Space>
            ),
          }] : []),
          ...(canReadTemplates ? [{
            key: 'templates',
            label: '工作流模板',
            children: (
              <Space direction="vertical" style={{ width: '100%' }} size="middle">
                <Form layout="inline" onFinish={(values) => loadTemplates(Object.fromEntries(Object.entries(values).filter(([, value]) => value)) as Record<string, string>)}>
                  <Form.Item name="business_app_code"><Input placeholder="业务应用" allowClear /></Form.Item>
                  <Form.Item name="graph_key"><Input placeholder="图标识" allowClear style={{ width: 260 }} /></Form.Item>
                  <Form.Item name="status"><Input placeholder="状态" allowClear /></Form.Item>
                  <Form.Item><Button type="primary" htmlType="submit">搜索</Button></Form.Item>
                  <Form.Item><Button htmlType="reset" onClick={() => loadTemplates()}>重置</Button></Form.Item>
                </Form>
                <Table size="small" dataSource={templates} columns={templateColumns} rowKey="id" loading={templateLoading} scroll={{ x: 1200 }} locale={{ emptyText: '暂无工作流模板' }} />
              </Space>
            ),
          }] : []),
          ...(canManageAgents ? [{
            key: 'agents',
            label: '智能体列表',
            children: (
              <Space direction="vertical" style={{ width: '100%' }} size="middle">
                <Form layout="inline" onFinish={(values) => loadAgents(Object.fromEntries(Object.entries(values).filter(([, value]) => value)) as Record<string, string>)}>
                  <Form.Item name="domain"><Input placeholder="领域" allowClear /></Form.Item>
                  <Form.Item name="status"><Input placeholder="状态" allowClear /></Form.Item>
                  <Form.Item><Button type="primary" htmlType="submit">搜索</Button></Form.Item>
                  <Form.Item><Button htmlType="reset" onClick={() => loadAgents()}>重置</Button></Form.Item>
                </Form>
                <Table size="small" dataSource={agents} columns={agentColumns} rowKey={(record: any) => record.id || record.agent_id} loading={agentLoading} scroll={{ x: 1100 }} locale={{ emptyText: '暂无智能体注册记录' }} />
              </Space>
            ),
          }] : []),
          ...(canManageTools ? [{
            key: 'tools',
            label: '工具列表',
            children: (
              <Space direction="vertical" style={{ width: '100%' }} size="middle">
                <Form layout="inline" onFinish={(values) => loadTools(Object.fromEntries(Object.entries(values).filter(([, value]) => value)) as Record<string, string>)}>
                  <Form.Item name="domain"><Input placeholder="领域" allowClear /></Form.Item>
                  <Form.Item name="risk_level"><Input placeholder="风险等级" allowClear /></Form.Item>
                  <Form.Item name="is_shared"><Select placeholder="共享范围" allowClear style={{ width: 120 }} options={[{ value: 'true', label: '共享' }, { value: 'false', label: '领域内' }]} /></Form.Item>
                  <Form.Item name="status"><Input placeholder="状态" allowClear /></Form.Item>
                  <Form.Item><Button type="primary" htmlType="submit">搜索</Button></Form.Item>
                  <Form.Item><Button htmlType="reset" onClick={() => loadTools()}>重置</Button></Form.Item>
                </Form>
                <Table size="small" dataSource={tools} columns={toolColumns} rowKey={(record: any) => getValue(record, 'id', 'ID') || getValue(record, 'tool_id', 'ToolID')} loading={toolLoading} scroll={{ x: 1100 }} locale={{ emptyText: '暂无工具注册记录' }} />
              </Space>
            ),
          }] : []),
        ]}
      />

      <Drawer title={selected?.title} open={!!selected} onClose={() => setSelected(null)} width={760}>
        {selected && (
          <Space direction="vertical" style={{ width: '100%' }} size="middle">
            <Descriptions column={1} size="small" bordered>
              {Object.entries(selected.record)
                .filter(([key]) => !selected.fields.includes(key))
                .map(([key, value]) => (
                  <Descriptions.Item key={key} label={key}>{String(value ?? '-')}</Descriptions.Item>
                ))}
            </Descriptions>
            {selected.fields.map((field) => (
              <pre key={field} style={{ whiteSpace: 'pre-wrap', background: '#f5f5f5', padding: 12, margin: 0 }}>
                {detailFieldLabels[field] || field}
                {'\n'}
                {prettyJSON(selected.record[field])}
              </pre>
            ))}
          </Space>
        )}
      </Drawer>
    </div>
  );
}
