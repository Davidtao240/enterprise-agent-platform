import { useEffect, useRef, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Alert, Button, Card, Descriptions, Drawer, Popconfirm, Space, Spin, Steps, Table, Tag, Typography } from 'antd';
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
import { tStatus } from '../utils/i18n';

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
  const [loadError, setLoadError] = useState('');
  const navigate = useNavigate();
  const hasPermission = useAuthStore((s) => s.hasPermission);
  const canReadApprovals = hasPermission('approval:read');
  const canDecideApproval = hasPermission('approval:decide');
  const canStart = hasPermission('workflow:start');
  const canCancel = hasPermission('workflow:cancel');
  const canRetry = hasPermission('workflow:retry');

  const fetchDetail = () => {
    if (!id) return Promise.resolve();
    setLoadError('');
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
    }).catch((err) => {
      setLoadError(err.response?.data?.error?.message || '流程详情加载失败');
      throw err;
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
  if (!instance) return <Alert type="warning" showIcon message="未找到流程实例" />;

  const currentNodeIdx = nodes.findIndex((n: any) =>
    ['running', 'waiting_review', 'pending'].includes(n.status),
  );
  const approvalByNodeId = new Map(approvals.map((task: any) => [task.NodeInstanceID || task.node_instance_id, task]));
  const nodeById = new Map(nodes.map((node: any) => [node.id, node]));
  const refreshAfterAction = () => fetchDetail();

  const runLogColumns = [
    { title: '图', dataIndex: 'graph_key', key: 'graph_key' },
    { title: '节点', key: 'node', render: (_: any, r: any) => nodeById.get(r.node_instance_id)?.name || r.node_instance_id },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (s: string) => <Tag color={s === 'succeeded' ? 'success' : s === 'failed' ? 'error' : 'processing'}>{tStatus(s)}</Tag>,
    },
    { title: '耗时', dataIndex: 'duration_ms', key: 'duration_ms', render: (v: number) => (v == null ? '-' : `${v}ms`) },
    { title: '输出 / 错误', key: 'summary', render: (_: any, r: any) => summarizeRunOutput(r.error_json || r.output_summary_json) },
    { title: '完成时间', dataIndex: 'finished_at', key: 'finished_at' },
    { title: '操作', key: 'detail', render: (_: any, r: any) => <Button size="small" onClick={() => setSelectedRun(r)}>详情</Button> },
  ];

  return (
    <div>
      <Space style={{ marginBottom: 16, justifyContent: 'space-between', width: '100%' }}>
        <Title level={4} style={{ margin: 0 }}>{instance.title}</Title>
        <Space>
          {instance.trace_id && (
            <Button onClick={() => navigate(`/audit-logs?trace_id=${encodeURIComponent(instance.trace_id)}`)}>审计链路</Button>
          )}
          {instance.status === 'draft' && canStart && (
            <Button type="primary" onClick={() => startWorkflow(instance.id).then(refreshAfterAction)}>启动</Button>
          )}
          {['running', 'waiting_review'].includes(instance.status) && canCancel && (
            <Popconfirm title="确认取消这个流程？" okText="确认" cancelText="返回" onConfirm={() => cancelWorkflow(instance.id).then(refreshAfterAction)}>
              <Button danger>取消</Button>
            </Popconfirm>
          )}
        </Space>
      </Space>
      {loadError && <Alert type="error" showIcon message={loadError} style={{ marginBottom: 16 }} />}
      <Card style={{ marginBottom: 16 }}>
        <Descriptions column={3} size="small">
          <Descriptions.Item label="状态"><Tag color={statusColor[instance.status]}>{tStatus(instance.status)}</Tag></Descriptions.Item>
          <Descriptions.Item label="业务应用">{instance.business_app_code}</Descriptions.Item>
          <Descriptions.Item label="模板">{instance.workflow_template_key}</Descriptions.Item>
          <Descriptions.Item label="追踪 ID">{instance.trace_id}</Descriptions.Item>
        </Descriptions>
      </Card>

      <Card title="流程节点">
        <Steps
          direction="vertical"
          current={currentNodeIdx}
          items={nodes.map((n: any) => ({
            title: `${n.name} (${n.node_type})`,
            description: (
              <Space>
                <Tag color={statusColor[n.status]}>{tStatus(n.status)}</Tag>
                {n.status === 'running' && <Spin size="small" />}
                {n.node_type === 'human_review' && n.status === 'waiting_review' && approvalByNodeId.get(n.id) && canDecideApproval && (
                  <Button
                    size="small"
                    type="primary"
                    onClick={() => navigate(`/approvals/${approvalByNodeId.get(n.id).ID || approvalByNodeId.get(n.id).id}`)}
                  >
                    审批
                  </Button>
                )}
                {n.error_json && <span style={{ color: 'red' }}>{typeof n.error_json === 'string' ? n.error_json : n.error_json.message}</span>}
                {n.status === 'failed' && canRetry && (
                  <Button size="small" onClick={() => retryWorkflowNode(instance.id, n.id).then(refreshAfterAction)}>
                    重试
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

      <Card title="智能体执行记录" style={{ marginTop: 16 }}>
        <Table
          size="small"
          dataSource={runLogs}
          columns={runLogColumns}
          rowKey={(r: any) => r.id || r.run_id}
          pagination={false}
          scroll={{ x: 1000 }}
          locale={{ emptyText: '暂无智能体执行记录' }}
        />
      </Card>

      <Drawer title="智能体执行详情" open={!!selectedRun} onClose={() => setSelectedRun(null)} width={720}>
        {selectedRun && (
          <Space direction="vertical" style={{ width: '100%' }} size="middle">
            <Descriptions column={1} size="small" bordered>
              <Descriptions.Item label="追踪 ID">{selectedRun.trace_id}</Descriptions.Item>
              <Descriptions.Item label="运行 ID">{selectedRun.run_id}</Descriptions.Item>
              <Descriptions.Item label="节点">{nodeById.get(selectedRun.node_instance_id)?.name || selectedRun.node_instance_id}</Descriptions.Item>
              <Descriptions.Item label="图">{selectedRun.graph_key}</Descriptions.Item>
              <Descriptions.Item label="状态"><Tag>{tStatus(selectedRun.status)}</Tag></Descriptions.Item>
              <Descriptions.Item label="耗时">{selectedRun.duration_ms == null ? '-' : `${selectedRun.duration_ms}ms`}</Descriptions.Item>
              <Descriptions.Item label="开始时间">{selectedRun.started_at || '-'}</Descriptions.Item>
              <Descriptions.Item label="完成时间">{selectedRun.finished_at || '-'}</Descriptions.Item>
            </Descriptions>
            <Card size="small" title="输出摘要">
              <pre style={{ whiteSpace: 'pre-wrap', margin: 0 }}>{formatJSON(selectedRun.output_summary_json)}</pre>
            </Card>
            <Card size="small" title="错误">
              <pre style={{ whiteSpace: 'pre-wrap', margin: 0 }}>{formatJSON(selectedRun.error_json)}</pre>
            </Card>
            <Card size="small" title="用量">
              <pre style={{ whiteSpace: 'pre-wrap', margin: 0 }}>{formatJSON(selectedRun.usage_json)}</pre>
            </Card>
          </Space>
        )}
      </Drawer>
    </div>
  );
}
