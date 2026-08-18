import { useCallback, useEffect, useMemo, useState } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import {
  Alert, Button, Card, Col, Descriptions, Row, Space, Spin, Tag, Timeline, Typography, message,
} from 'antd';
import { ReloadOutlined, ThunderboltOutlined } from '@ant-design/icons';
import {
  getRunDetail, getTrace, getApprovalTasks, approveTask, rejectTask, createReplay,
} from '../services/api';
import StatusBadge from '../components/StatusBadge';
import PayloadViewer from '../components/PayloadViewer';
import { useAuthStore } from '../store/auth';

const { Title, Text } = Typography;

// ── 后端契约类型(M1 DurableRun / M5-A Trace) ──

interface DurableRun {
  id: string;
  thread_id: string;
  trace_id: string;
  workflow_instance_id?: string;
  node_instance_id?: string;
  graph_key: string;
  graph_version: string;
  status: string;
  attempt: number;
  output_summary_json?: string;
  usage_json?: string;
  error_json?: string;
  started_at?: string;
  finished_at?: string;
  created_at: string;
  updated_at: string;
  metadata_json?: string;
}

interface AgentRunStep {
  id: string;
  sequence: number;
  attempt: number;
  step_type: string;
  name?: string;
  status: string;
  input_summary_json?: string;
  output_summary_json?: string;
  usage_json?: string;
  error_json?: string;
  started_at?: string;
  created_at: string;
}

interface RuntimeEvent {
  event_id: string;
  sequence: number;
  type: string;
  payload_json?: string;
  occurred_at: string;
}

interface TraceEvent {
  id: string;
  layer: string;
  event_type: string;
  payload_json?: string;
  timestamp: string;
  duration_ms?: number;
}

interface ApprovalTask {
  id: string;
  title: string;
  status: string;
  durable_run_id?: string;
  tool_call_id?: string;
}

// 时间线节点(Step / RuntimeEvent / TraceEvent 归一化)。
interface TimelineNode {
  key: string;
  time: string;
  kind: 'step' | 'event' | 'trace';
  layer?: string;
  label: string;
  status?: string;
  payloads: { title: string; value?: string | null }[];
}

const LAYER_COLOR: Record<string, string> = {
  L1: 'geekblue',
  L2: 'blue',
  L3: 'purple',
  L4: 'cyan',
  L5: 'gold',
  L6: 'magenta',
};

const ACTIVE_STATUSES = ['queued', 'running'];

function fmtDuration(start?: string, end?: string): string {
  if (!start) return '—';
  const endTime = end ? new Date(end).getTime() : Date.now();
  const ms = endTime - new Date(start).getTime();
  if (ms < 0) return '—';
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.floor(ms / 60000)}m${Math.floor((ms % 60000) / 1000)}s`;
}

export default function RunDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const hasPermission = useAuthStore((s) => s.hasPermission);

  const [loading, setLoading] = useState(true);
  const [run, setRun] = useState<DurableRun | null>(null);
  const [steps, setSteps] = useState<AgentRunStep[]>([]);
  const [events, setEvents] = useState<RuntimeEvent[]>([]);
  const [traceEvents, setTraceEvents] = useState<TraceEvent[]>([]);
  const [pendingTask, setPendingTask] = useState<ApprovalTask | null>(null);
  const [selected, setSelected] = useState<TimelineNode | null>(null);

  const load = useCallback(async () => {
    if (!id) return;
    try {
      const { data } = await getRunDetail(id);
      const detail = data.data;
      setRun(detail.run);
      setSteps(detail.steps || []);
      setEvents(detail.events || []);
      if (detail.run?.trace_id && hasPermission('trace:read')) {
        getTrace(detail.run.trace_id)
          .then(({ data: t }) => setTraceEvents(t.data?.events || []))
          .catch(() => setTraceEvents([]));
      }
      if (detail.run?.status === 'waiting_human' && hasPermission('approval:read')) {
        const { data: a } = await getApprovalTasks({ status: 'pending', page: '1', page_size: '100' });
        const task = (a.data || []).find((t: ApprovalTask) => t.durable_run_id === id) || null;
        setPendingTask(task);
      } else {
        setPendingTask(null);
      }
    } finally {
      setLoading(false);
    }
  }, [id, hasPermission]);

  useEffect(() => {
    load();
  }, [load]);

  // Running 态自动轮询(Spec §3.1)
  useEffect(() => {
    if (!run || !ACTIVE_STATUSES.includes(run.status)) return;
    const timer = setInterval(load, 5000);
    return () => clearInterval(timer);
  }, [run, load]);

  // ── 时间线归一化:Steps + RuntimeEvents + TraceEvents 按时间排序 ──
  const nodes: TimelineNode[] = useMemo(() => {
    const result: TimelineNode[] = [];
    (steps || []).forEach((s) => {
      result.push({
        key: `step-${s.id}`,
        time: s.started_at || s.created_at,
        kind: 'step',
        label: `Step #${s.sequence} ${s.step_type}${s.name ? ` · ${s.name}` : ''}`,
        status: s.status,
        payloads: [
          { title: 'Input', value: s.input_summary_json },
          { title: 'Output', value: s.output_summary_json },
          { title: 'Usage', value: s.usage_json },
          { title: 'Error', value: s.error_json },
        ],
      });
    });
    (events || []).forEach((e) => {
      result.push({
        key: `event-${e.event_id}`,
        time: e.occurred_at,
        kind: 'event',
        label: e.type,
        payloads: [{ title: 'Payload', value: e.payload_json }],
      });
    });
    (traceEvents || []).forEach((t) => {
      result.push({
        key: `trace-${t.id}`,
        time: t.timestamp,
        kind: 'trace',
        layer: t.layer,
        label: `${t.event_type}${t.duration_ms != null ? ` (${t.duration_ms}ms)` : ''}`,
        payloads: [{ title: 'Trace Payload', value: t.payload_json }],
      });
    });
    return result.sort((a, b) => new Date(a.time).getTime() - new Date(b.time).getTime());
  }, [steps, events, traceEvents]);

  const dotColor = (n: TimelineNode): string => {
    if (n.kind === 'trace') {
      if (n.layer === 'L6') return 'red';
      if (n.layer === 'L4') return 'cyan';
      return 'blue';
    }
    if (n.kind === 'event') return 'gray';
    switch (n.status) {
      case 'succeeded': return 'green';
      case 'failed': return 'red';
      case 'running': return 'blue';
      default: return 'gray';
    }
  };

  const handleDecision = async (approve: boolean, comment: string) => {
    if (!pendingTask) return;
    if (approve) await approveTask(pendingTask.id, comment);
    else await rejectTask(pendingTask.id, comment);
    message.success(approve ? '已批准' : '已拒绝');
    load();
  };

  const handleReplay = async () => {
    if (!run) return;
    const { data } = await createReplay({ source_run_id: run.id });
    message.success(`重放会话已创建: ${data.data.id}`);
    navigate('/experiments');
  };

  const metadata = useMemo(() => {
    if (!run?.metadata_json) return null;
    try { return JSON.parse(run.metadata_json); } catch { return null; }
  }, [run?.metadata_json]);

  if (loading) return <Spin size="large" style={{ display: 'block', margin: '100px auto' }} />;
  if (!run) return <Alert type="error" message="Run 不存在或无权访问" showIcon />;

  return (
    <div>
      <Title level={4}>
        Run 详情 <Text copyable style={{ fontSize: 14 }}>{run.id}</Text>
      </Title>
      <Row gutter={16}>
        <Col span={16}>
          <Card size="small" title="基本信息" style={{ marginBottom: 16 }}>
            <Descriptions size="small" column={2}>
              <Descriptions.Item label="状态"><StatusBadge status={run.status} /></Descriptions.Item>
              <Descriptions.Item label="Graph">{run.graph_key} @ {run.graph_version}</Descriptions.Item>
              <Descriptions.Item label="开始时间">{run.started_at || '—'}</Descriptions.Item>
              <Descriptions.Item label="耗时">{fmtDuration(run.started_at, run.finished_at)}</Descriptions.Item>
              <Descriptions.Item label="Attempt">{run.attempt}</Descriptions.Item>
              <Descriptions.Item label="创建时间">{run.created_at}</Descriptions.Item>
            </Descriptions>
            {metadata && (
              <Space style={{ marginTop: 8 }}>
                {metadata.shadow && <Tag color="purple">Shadow</Tag>}
                {metadata.replay && <Tag color="geekblue">Replay</Tag>}
                {metadata.canary_release_id && <Tag color="orange">Canary: {metadata.canary_release_id.slice(0, 8)}</Tag>}
              </Space>
            )}
          </Card>

          {run.status === 'waiting_human' && (
            <Alert
              type="warning"
              showIcon
              message="等待人工审批"
              description={
                pendingTask ? (
                  <Space direction="vertical">
                    <Text>{pendingTask.title}</Text>
                    <Space>
                      {hasPermission('approval:decide') && (
                        <>
                          <Button type="primary" size="small" onClick={() => handleDecision(true, 'approved via workbench')}>批准</Button>
                          <Button danger size="small" onClick={() => handleDecision(false, 'rejected via workbench')}>拒绝</Button>
                        </>
                      )}
                      <Button size="small" onClick={() => navigate(`/approvals/${pendingTask.id}`)}>查看审批详情</Button>
                    </Space>
                  </Space>
                ) : '未找到关联的待审批任务(可能已处理或由 Tool Call 审批承载)。'
              }
              style={{ marginBottom: 16 }}
            />
          )}

          <Card
            size="small"
            title={`时间线 (${nodes.length} 节点)`}
            extra={<Button size="small" icon={<ReloadOutlined />} onClick={load}>刷新</Button>}
          >
            {nodes.length === 0 ? (
              <Text type="secondary">暂无时间线数据</Text>
            ) : (
              <Timeline
                items={nodes.map((n) => ({
                  key: n.key,
                  color: dotColor(n),
                  children: (
                    <div
                      onClick={() => setSelected(n)}
                      style={{ cursor: 'pointer', padding: '2px 4px', background: selected?.key === n.key ? 'rgba(22,119,255,0.08)' : undefined, borderRadius: 4 }}
                    >
                      <Space size={6} wrap>
                        {n.layer && <Tag color={LAYER_COLOR[n.layer]}>{n.layer}</Tag>}
                        {!n.layer && n.kind === 'step' && <Tag>Step</Tag>}
                        {!n.layer && n.kind === 'event' && <Tag color="default">Event</Tag>}
                        <Text strong={n.layer === 'L6'} style={n.layer === 'L6' ? { color: '#cf1322' } : undefined}>{n.label}</Text>
                        {n.status && <StatusBadge status={n.status} />}
                        <Text type="secondary" style={{ fontSize: 12 }}>{new Date(n.time).toLocaleString()}</Text>
                      </Space>
                    </div>
                  ),
                }))}
              />
            )}
          </Card>
        </Col>

        <Col span={8}>
          <Card size="small" title="Payload Inspector" style={{ marginBottom: 16 }}>
            {!selected ? (
              <Text type="secondary">点击左侧时间线节点查看详情</Text>
            ) : (
              <Space direction="vertical" style={{ width: '100%' }} size="middle">
                <Text strong>{selected.label}</Text>
                {selected.payloads.map((p) => (
                  <div key={p.title}>
                    <Text type="secondary" style={{ fontSize: 12 }}>{p.title}</Text>
                    <PayloadViewer value={p.value} />
                  </div>
                ))}
              </Space>
            )}
          </Card>

          <Card size="small" title="关联信息" style={{ marginBottom: 16 }}>
            <Descriptions size="small" column={1}>
              <Descriptions.Item label="Trace ID">
                <Text copyable style={{ fontSize: 12 }}>{run.trace_id}</Text>
              </Descriptions.Item>
              <Descriptions.Item label="Thread">{run.thread_id}</Descriptions.Item>
              <Descriptions.Item label="Workflow 实例">
                {run.workflow_instance_id
                  ? <Link to={`/workflows/${run.workflow_instance_id}`}>{run.workflow_instance_id.slice(0, 8)}…</Link>
                  : '—'}
              </Descriptions.Item>
              <Descriptions.Item label="Node 实例">{run.node_instance_id?.slice(0, 8) || '—'}</Descriptions.Item>
            </Descriptions>
          </Card>

          <Card size="small" title="快速操作">
            <Space direction="vertical" style={{ width: '100%' }}>
              {hasPermission('experiment:manage') && (
                <Button block icon={<ThunderboltOutlined />} onClick={handleReplay}>
                  重放此 Run
                </Button>
              )}
              <Text type="secondary" style={{ fontSize: 12 }}>
                Run 输出 / 错误:
              </Text>
              <PayloadViewer value={run.output_summary_json || run.error_json} />
            </Space>
          </Card>
        </Col>
      </Row>
    </div>
  );
}
