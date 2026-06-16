import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Button, Card, Descriptions, Space, Spin, Steps, Tag, Typography } from 'antd';
import { getWorkflowInstance, getWorkflowNodes } from '../services/api';

const { Title } = Typography;

const statusColor: Record<string, string> = {
  pending: 'default',
  running: 'processing',
  succeeded: 'success',
  failed: 'error',
  skipped: 'default',
  waiting_review: 'warning',
  cancelled: 'default',
};

export default function WorkflowDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [instance, setInstance] = useState<any>(null);
  const [nodes, setNodes] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();

  useEffect(() => {
    if (!id) return;
    Promise.all([
      getWorkflowInstance(id),
      getWorkflowNodes(id),
    ])
      .then(([instRes, nodesRes]) => {
        setInstance(instRes.data.data);
        setNodes(nodesRes.data.data);
      })
      .finally(() => setLoading(false));
  }, [id]);

  if (loading) return <Spin size="large" style={{ display: 'block', margin: '100px auto' }} />;
  if (!instance) return <div>Not found</div>;

  const currentNodeIdx = nodes.findIndex((n: any) =>
    ['running', 'waiting_review', 'pending'].includes(n.status),
  );

  return (
    <div>
      <Title level={4}>{instance.title}</Title>
      <Card style={{ marginBottom: 16 }}>
        <Descriptions column={3} size="small">
          <Descriptions.Item label="Status"><Tag color={statusColor[instance.status]}>{instance.status}</Tag></Descriptions.Item>
          <Descriptions.Item label="Business App">{instance.business_app_code}</Descriptions.Item>
          <Descriptions.Item label="Template">{instance.workflow_template_key}</Descriptions.Item>
        </Descriptions>
      </Card>

      <Card title="Workflow Nodes">
        <Steps
          direction="vertical"
          current={currentNodeIdx}
          items={nodes.map((n: any) => ({
            title: `${n.name} (${n.node_type})`,
            description: (
              <Space>
                <Tag color={statusColor[n.status]}>{n.status}</Tag>
                {n.node_type === 'human_review' && n.status === 'waiting_review' && (
                  <Button size="small" type="primary" onClick={() => navigate(`/approvals/${n.id}`)}>
                    Review
                  </Button>
                )}
                {n.error_json && <span style={{ color: 'red' }}>{n.error_json.message}</span>}
              </Space>
            ),
            status: n.status === 'failed' ? 'error' : n.status === 'running' ? 'process' : n.status === 'succeeded' ? 'finish' : 'wait',
          }))}
        />
      </Card>
    </div>
  );
}
