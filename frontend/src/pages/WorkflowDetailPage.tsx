import { useEffect, useRef, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Button, Card, Descriptions, Drawer, Popconfirm, Space, Spin, Steps, Table, Tag, Typography } from 'antd';
import {
  cancelWorkflow,
  getAgentRunLogs,
  getApprovalTasks,
  getWorkflowInstance,
  getWorkflowNodes,
  retryWorkflowNode,
  startWorkflow,
} from '../services/api';
import { useAuthStore } from '../store/auth';

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

function parseJSON(raw: any): any {
  if (!raw) return null;
  if (typeof raw !== 'string') return raw;
  try {
    return JSON.parse(raw);
  } catch {
    return raw;
  }
}

function summarizeRunOutput(raw: any): string {
  const obj = parseJSON(raw);
  if (!obj) return '';
  if (typeof obj === 'string') return obj;
  return obj.summary || obj.title || obj.error?.message || JSON.stringify(obj).slice(0, 160);
}

function formatJSON(raw: any): string {
  const obj = parseJSON(raw);
  if (!obj) return '{}';
  if (typeof obj === 'string') return obj;
  return JSON.stringify(obj, null, 2);
}

export default function WorkflowDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [instance, setInstance] = useState<any>(null);
  const [nodes, setNodes] = useState<any[]>([]);
  const [approvals, setApprovals] = useState<any[]>([]);
  const [runLogs, setRunLogs] = useState<any[]>([]);
  const [selectedRun, setSelectedRun] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();
  const hasPermission = useAuthStore((s) => s.hasPermission);
  const canReadApprovals = hasPermission('approval:read');
  const canDecideApproval = hasPermission('approval:decide');
  const canStart = hasPermission('workflow:start');
  const canCancel = hasPermission('workflow:cancel');
  const canRetry = hasPermission('workflow:retry');

  const fetchDetail = () => {
    if (!id) return Promise.resolve();
    return Promise.all([
      getWorkflowInstance(id),
      getWorkflowNodes(id),
      canReadApprovals ? getApprovalTasks({ workflow_instance_id: id }) : Promise.resolve({ data: { data: [] } }),
      getAgentRunLogs({ workflow_instance_id: id }),
    ]).then(([instRes, nodesRes, approvalRes, runLogRes]) => {
      setInstance(instRes.data.data);
      setNodes(nodesRes.data.data);
      setApprovals(approvalRes.data.data || []);
      setRunLogs(runLogRes.data.data || []);
    });
  };

  useEffect(() => {
    if (!id) return;
    fetchDetail()
      .finally(() => setLoading(false));
  }, [id, canReadApprovals]);

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
      fetchDetail();
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
  const nodeById = new Map(nodes.map((node: any) => [node.id, node]));
  const refreshAfterAction = () => fetchDetail();

  const runLogColumns = [
    { title: 'Graph', dataIndex: 'graph_key', key: 'graph_key' },
    { title: 'Node', key: 'node', render: (_: any, r: any) => nodeById.get(r.node_instance_id)?.name || r.node_instance_id },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      render: (s: string) => <Tag color={s === 'succeeded' ? 'success' : s === 'failed' ? 'error' : 'processing'}>{s}</Tag>,
    },
    { title: 'Duration', dataIndex: 'duration_ms', key: 'duration_ms', render: (v: number) => (v == null ? '-' : `${v}ms`) },
    { title: 'Output / Error', key: 'summary', render: (_: any, r: any) => summarizeRunOutput(r.error_json || r.output_summary_json) },
    { title: 'Finished', dataIndex: 'finished_at', key: 'finished_at' },
    { title: '', key: 'detail', render: (_: any, r: any) => <Button size="small" onClick={() => setSelectedRun(r)}>Detail</Button> },
  ];

  return (
    <div>
      <Space style={{ marginBottom: 16, justifyContent: 'space-between', width: '100%' }}>
        <Title level={4} style={{ margin: 0 }}>{instance.title}</Title>
        <Space>
          {instance.trace_id && (
            <Button onClick={() => navigate(`/audit-logs?trace_id=${encodeURIComponent(instance.trace_id)}`)}>Audit Trace</Button>
          )}
          {instance.status === 'draft' && canStart && (
            <Button type="primary" onClick={() => startWorkflow(instance.id).then(refreshAfterAction)}>Start</Button>
          )}
          {['running', 'waiting_review'].includes(instance.status) && canCancel && (
            <Popconfirm title="Cancel this workflow?" onConfirm={() => cancelWorkflow(instance.id).then(refreshAfterAction)}>
              <Button danger>Cancel</Button>
            </Popconfirm>
          )}
        </Space>
      </Space>
      <Card style={{ marginBottom: 16 }}>
        <Descriptions column={3} size="small">
          <Descriptions.Item label="Status"><Tag color={statusColor[instance.status]}>{instance.status}</Tag></Descriptions.Item>
          <Descriptions.Item label="Business App">{instance.business_app_code}</Descriptions.Item>
          <Descriptions.Item label="Template">{instance.workflow_template_key}</Descriptions.Item>
          <Descriptions.Item label="Trace ID">{instance.trace_id}</Descriptions.Item>
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
                {n.node_type === 'human_review' && n.status === 'waiting_review' && approvalByNodeId.get(n.id) && canDecideApproval && (
                  <Button
                    size="small"
                    type="primary"
                    onClick={() => navigate(`/approvals/${approvalByNodeId.get(n.id).ID || approvalByNodeId.get(n.id).id}`)}
                  >
                    Review
                  </Button>
                )}
                {n.error_json && <span style={{ color: 'red' }}>{typeof n.error_json === 'string' ? n.error_json : n.error_json.message}</span>}
                {n.status === 'failed' && canRetry && (
                  <Button size="small" onClick={() => retryWorkflowNode(instance.id, n.id).then(refreshAfterAction)}>
                    Retry
                  </Button>
                )}
                {n.status === 'succeeded' && getNodeSummary(n) && (
                  <span style={{ color: '#595959', fontSize: 12 }}>{getNodeSummary(n)}</span>
                )}
              </Space>
            ),
            status: n.status === 'failed' ? 'error' : n.status === 'running' ? 'process' : n.status === 'succeeded' ? 'finish' : 'wait',
          }))}
        />
      </Card>

      <Card title="Agent Run Logs" style={{ marginTop: 16 }}>
        <Table
          size="small"
          dataSource={runLogs}
          columns={runLogColumns}
          rowKey={(r: any) => r.id || r.run_id}
          pagination={false}
          scroll={{ x: 1000 }}
        />
      </Card>

      <Drawer title="Agent Run Detail" open={!!selectedRun} onClose={() => setSelectedRun(null)} width={720}>
        {selectedRun && (
          <Space direction="vertical" style={{ width: '100%' }} size="middle">
            <Descriptions column={1} size="small" bordered>
              <Descriptions.Item label="Trace ID">{selectedRun.trace_id}</Descriptions.Item>
              <Descriptions.Item label="Run ID">{selectedRun.run_id}</Descriptions.Item>
              <Descriptions.Item label="Node">{nodeById.get(selectedRun.node_instance_id)?.name || selectedRun.node_instance_id}</Descriptions.Item>
              <Descriptions.Item label="Graph">{selectedRun.graph_key}</Descriptions.Item>
              <Descriptions.Item label="Status"><Tag>{selectedRun.status}</Tag></Descriptions.Item>
              <Descriptions.Item label="Duration">{selectedRun.duration_ms == null ? '-' : `${selectedRun.duration_ms}ms`}</Descriptions.Item>
              <Descriptions.Item label="Started At">{selectedRun.started_at || '-'}</Descriptions.Item>
              <Descriptions.Item label="Finished At">{selectedRun.finished_at || '-'}</Descriptions.Item>
            </Descriptions>
            <Card size="small" title="Output Summary">
              <pre style={{ whiteSpace: 'pre-wrap', margin: 0 }}>{formatJSON(selectedRun.output_summary_json)}</pre>
            </Card>
            <Card size="small" title="Error">
              <pre style={{ whiteSpace: 'pre-wrap', margin: 0 }}>{formatJSON(selectedRun.error_json)}</pre>
            </Card>
            <Card size="small" title="Usage">
              <pre style={{ whiteSpace: 'pre-wrap', margin: 0 }}>{formatJSON(selectedRun.usage_json)}</pre>
            </Card>
          </Space>
        )}
      </Drawer>
    </div>
  );
}
