import { useEffect, useRef, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Button, Card, Descriptions, Space, Spin, Steps, Tag, Typography } from 'antd';
import { getApprovalTasks, getWorkflowInstance, getWorkflowNodes } from '../services/api';

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

/** Parse output_json (string or object) and extract summary text. */
function getNodeSummary(n: any): string | null {
  const raw = n.output_json;
  if (!raw) return null;
  const obj = typeof raw === 'string' ? (() => { try { return JSON.parse(raw); } catch { return null; } })() : raw;
  if (!obj) return null;
  return obj.summary || obj.title || null;
}

export default function WorkflowDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [instance, setInstance] = useState<any>(null);
  const [nodes, setNodes] = useState<any[]>([]);
  const [approvals, setApprovals] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();

  useEffect(() => {
    if (!id) return;
    Promise.all([
      getWorkflowInstance(id),
      getWorkflowNodes(id),
      getApprovalTasks({ workflow_instance_id: id }),
    ])
      .then(([instRes, nodesRes, approvalRes]) => {
        setInstance(instRes.data.data);
        setNodes(nodesRes.data.data);
        setApprovals(approvalRes.data.data || []);
      })
      .finally(() => setLoading(false));
  }, [id]);

  // ── Polling: 当 instance 处于活跃状态时，每 3 秒刷新一次 ──
  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null);
  useEffect(() => {
    const isActive = instance && ['running', 'waiting_review'].includes(instance.status);
    if (!isActive || !id) {
      if (pollingRef.current) clearInterval(pollingRef.current);
      pollingRef.current = null;
      return;
    }
    pollingRef.current = setInterval(() => {
      Promise.all([
        getWorkflowInstance(id),
        getWorkflowNodes(id),
        getApprovalTasks({ workflow_instance_id: id }),
      ]).then(([instRes, nodesRes, approvalRes]) => {
        setInstance(instRes.data.data);
        setNodes(nodesRes.data.data);
        setApprovals(approvalRes.data.data || []);
      });
    }, 3000);
    return () => {
      if (pollingRef.current) clearInterval(pollingRef.current);
    };
  }, [instance?.status, id]);

  if (loading) return <Spin size="large" style={{ display: 'block', margin: '100px auto' }} />;
  if (!instance) return <div>Not found</div>;

  const currentNodeIdx = nodes.findIndex((n: any) =>
    ['running', 'waiting_review', 'pending'].includes(n.status),
  );
  const approvalByNodeId = new Map(approvals.map((task: any) => [task.NodeInstanceID || task.node_instance_id, task]));

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
                {n.status === 'running' && <Spin size="small" />}
                {n.node_type === 'human_review' && n.status === 'waiting_review' && approvalByNodeId.get(n.id) && (
                  <Button
                    size="small"
                    type="primary"
                    onClick={() => navigate(`/approvals/${approvalByNodeId.get(n.id).ID || approvalByNodeId.get(n.id).id}`)}
                  >
                    Review
                  </Button>
                )}
                {n.error_json && <span style={{ color: 'red' }}>{typeof n.error_json === 'string' ? n.error_json : n.error_json.message}</span>}
                {n.status === 'succeeded' && getNodeSummary(n) && (
                  <span style={{ color: '#595959', fontSize: 12 }}>{getNodeSummary(n)}</span>
                )}
              </Space>
            ),
            status: n.status === 'failed' ? 'error' : n.status === 'running' ? 'process' : n.status === 'succeeded' ? 'finish' : 'wait',
          }))}
        />
      </Card>
    </div>
  );
}
