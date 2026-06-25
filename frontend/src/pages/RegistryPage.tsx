import { useEffect, useState } from 'react';
import { Card, Col, Row, Table, Tag, Typography } from 'antd';
import { getAgents, getTools } from '../services/api';
import { useAuthStore } from '../store/auth';

const { Title } = Typography;

function getValue(record: any, snakeKey: string, camelKey: string) {
  return record[snakeKey] ?? record[camelKey];
}

export default function RegistryPage() {
  const hasPermission = useAuthStore((s) => s.hasPermission);
  const canManageAgents = hasPermission('agent:manage');
  const canManageTools = hasPermission('tool:manage');
  const [agents, setAgents] = useState<any[]>([]);
  const [tools, setTools] = useState<any[]>([]);
  const [agentLoading, setAgentLoading] = useState(false);
  const [toolLoading, setToolLoading] = useState(false);

  useEffect(() => {
    if (canManageAgents) {
      setAgentLoading(true);
      getAgents()
        .then(({ data }) => setAgents(data.data || []))
        .finally(() => setAgentLoading(false));
    }
    if (canManageTools) {
      setToolLoading(true);
      getTools()
        .then(({ data }) => setTools(data.data || []))
        .finally(() => setToolLoading(false));
    }
  }, [canManageAgents, canManageTools]);

  const agentColumns = [
    { title: 'Agent', dataIndex: 'agent_id', key: 'agent_id' },
    { title: 'Name', dataIndex: 'name', key: 'name' },
    { title: 'Domain', dataIndex: 'domain', key: 'domain' },
    { title: 'Scope', dataIndex: 'reusable_scope', key: 'reusable_scope' },
    { title: 'Status', dataIndex: 'status', key: 'status', render: (status: string) => <Tag color="success">{status}</Tag> },
  ];

  const toolColumns = [
    { title: 'Tool', key: 'tool_id', render: (_: any, record: any) => getValue(record, 'tool_id', 'ToolID') },
    { title: 'Name', key: 'name', render: (_: any, record: any) => getValue(record, 'name', 'Name') },
    { title: 'Domain', key: 'domain', render: (_: any, record: any) => getValue(record, 'domain', 'Domain') },
    { title: 'Risk', key: 'risk', render: (_: any, record: any) => <Tag>{getValue(record, 'risk_level', 'RiskLevel')}</Tag> },
    { title: 'Status', key: 'status', render: (_: any, record: any) => <Tag color="success">{getValue(record, 'status', 'Status')}</Tag> },
  ];

  return (
    <div>
      <Title level={4}>Registry</Title>
      <Row gutter={[16, 16]}>
        {canManageAgents && (
          <Col xs={24}>
            <Card title="Agents">
              <Table
                size="small"
                dataSource={agents}
                columns={agentColumns}
                rowKey={(record: any) => record.id || record.agent_id}
                loading={agentLoading}
                pagination={false}
              />
            </Card>
          </Col>
        )}
        {canManageTools && (
          <Col xs={24}>
            <Card title="Tools">
              <Table
                size="small"
                dataSource={tools}
                columns={toolColumns}
                rowKey={(record: any) => getValue(record, 'id', 'ID') || getValue(record, 'tool_id', 'ToolID')}
                loading={toolLoading}
                pagination={false}
              />
            </Card>
          </Col>
        )}
      </Row>
    </div>
  );
}
