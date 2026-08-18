import { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Button, Card, Descriptions, Drawer, Modal, Select, Space, Table, Tabs, Tag, Typography, message,
} from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { getOutboxEntries, compensateOutbox, getDeadLetterToolCalls } from '../services/api';
import StatusBadge from '../components/StatusBadge';
import PayloadViewer from '../components/PayloadViewer';
import { useAuthStore } from '../store/auth';

const { Title, Text } = Typography;

// OutboxEntry 模型(M3 契约)。
interface OutboxEntry {
  id: string;
  tool_call_id: string;
  connector_code: string;
  operation: string;
  payload_json: string;
  state: string;
  external_request_id?: string;
  external_object_id?: string;
  attempts: number;
  next_attempt_at: string;
  last_error?: string;
  created_at: string;
  updated_at: string;
}

interface DeadLetterToolCall {
  id: string;
  tool_id: string;
  status: string;
  retry_count: number;
  error_json?: string;
  run_id: string;
  updated_at: string;
}

const STATE_OPTIONS = [
  'pending', 'delivering', 'delivered', 'compensate_pending', 'compensated', 'dead_letter',
].map((s) => ({ value: s, label: s }));

export default function OpsOutboxPage() {
  const navigate = useNavigate();
  const hasPermission = useAuthStore((s) => s.hasPermission);
  const canReadDLQ = hasPermission('tool:read');

  const [entries, setEntries] = useState<OutboxEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [state, setState] = useState('');
  const [selected, setSelected] = useState<OutboxEntry | null>(null);
  const [compensating, setCompensating] = useState<OutboxEntry | null>(null);

  const [dlq, setDlq] = useState<DeadLetterToolCall[]>([]);
  const [dlqLoading, setDlqLoading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const params: Record<string, string> = { limit: '50' };
      if (state) params.state = state;
      const { data } = await getOutboxEntries(params);
      setEntries(data.data.items || []);
    } finally {
      setLoading(false);
    }
  }, [state]);

  const loadDlq = useCallback(async () => {
    if (!canReadDLQ) return;
    setDlqLoading(true);
    try {
      const { data } = await getDeadLetterToolCalls({ limit: '100' });
      setDlq(data.data.items || []);
    } finally {
      setDlqLoading(false);
    }
  }, [canReadDLQ]);

  useEffect(() => {
    load();
  }, [load]);

  const confirmCompensate = async () => {
    if (!compensating) return;
    try {
      await compensateOutbox(compensating.id, 'manual reconcile via workbench');
      message.success('已触发人工补偿');
      setCompensating(null);
      load();
    } catch {
      // 错误提示由 api 拦截器统一处理
    }
  };

  const outboxColumns = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 130,
      render: (v: string) => <Text copyable style={{ fontSize: 12 }}>{v.slice(0, 8)}…</Text>,
    },
    { title: '连接器', dataIndex: 'connector_code', width: 120 },
    { title: '操作', dataIndex: 'operation', width: 100 },
    { title: '状态', dataIndex: 'state', width: 140, render: (v: string) => <StatusBadge status={v} /> },
    { title: '尝试', dataIndex: 'attempts', width: 70 },
    { title: '下次投递', dataIndex: 'next_attempt_at', width: 170, render: (v: string) => new Date(v).toLocaleString() },
    {
      title: '关联 ToolCall',
      dataIndex: 'tool_call_id',
      width: 120,
      render: (v: string) => (
        <a onClick={() => navigate(`/explore/tool-calls`)}>{v.slice(0, 8)}…</a>
      ),
    },
    { title: '更新时间', dataIndex: 'updated_at', width: 170, render: (v: string) => new Date(v).toLocaleString() },
    {
      title: '操作',
      width: 80,
      render: (_: unknown, row: OutboxEntry) => <Button size="small" onClick={() => setSelected(row)}>详情</Button>,
    },
  ];

  const dlqColumns = [
    {
      title: 'Tool Call ID',
      dataIndex: 'id',
      width: 150,
      render: (v: string) => <Text copyable style={{ fontSize: 12 }}>{v.slice(0, 8)}…</Text>,
    },
    { title: 'Tool', dataIndex: 'tool_id', width: 160 },
    { title: '状态', dataIndex: 'status', width: 100, render: (v: string) => <StatusBadge status={v} /> },
    { title: '重试次数', dataIndex: 'retry_count', width: 90 },
    {
      title: 'Run',
      dataIndex: 'run_id',
      width: 110,
      render: (v: string) => <a onClick={() => navigate(`/runs/${v}`)}>{v.slice(0, 8)}…</a>,
    },
    { title: '更新时间', dataIndex: 'updated_at', width: 170, render: (v: string) => new Date(v).toLocaleString() },
  ];

  const items = [
    {
      key: 'outbox',
      label: 'Outbox 监控',
      children: (
        <Card size="small">
          <Space wrap style={{ marginBottom: 12 }}>
            <Select
              allowClear
              placeholder="状态过滤"
              style={{ width: 180 }}
              options={STATE_OPTIONS}
              value={state || undefined}
              onChange={(v) => setState(v || '')}
            />
            <Button icon={<ReloadOutlined />} onClick={load}>刷新</Button>
          </Space>
          <Table
            rowKey="id"
            size="small"
            loading={loading}
            dataSource={entries}
            columns={outboxColumns}
            pagination={{ pageSize: 20, showSizeChanger: false }}
            scroll={{ x: 1100 }}
          />
        </Card>
      ),
    },
    ...(canReadDLQ
      ? [
          {
            key: 'dlq',
            label: '死信队列 (DLQ)',
            children: (
              <Card size="small">
                <Space style={{ marginBottom: 12 }}>
                  <Button icon={<ReloadOutlined />} onClick={loadDlq}>刷新</Button>
                </Space>
                <Table
                  rowKey="id"
                  size="small"
                  loading={dlqLoading}
                  dataSource={dlq}
                  columns={dlqColumns}
                  pagination={{ pageSize: 20, showSizeChanger: false }}
                  scroll={{ x: 800 }}
                />
              </Card>
            ),
          },
        ]
      : []),
  ];

  return (
    <div>
      <Title level={4}>可靠性运维 · Outbox</Title>
      <Tabs items={items} onChange={(key) => key === 'dlq' && loadDlq()} />

      <Drawer
        title="Outbox 详情"
        width={520}
        open={!!selected}
        onClose={() => setSelected(null)}
        extra={
          selected && (
            <Space>
              {['delivered', 'compensated'].includes(selected.state) ? (
                <Tag color="success">终态,不可补偿</Tag>
              ) : (
                <Button danger size="small" onClick={() => setCompensating(selected)}>
                  人工补偿
                </Button>
              )}
            </Space>
          )
        }
      >
        {selected && (
          <Space direction="vertical" style={{ width: '100%' }} size="middle">
            <Descriptions size="small" column={1} bordered>
              <Descriptions.Item label="ID"><Text copyable style={{ fontSize: 11 }}>{selected.id}</Text></Descriptions.Item>
              <Descriptions.Item label="连接器">{selected.connector_code}</Descriptions.Item>
              <Descriptions.Item label="操作">{selected.operation}</Descriptions.Item>
              <Descriptions.Item label="状态"><StatusBadge status={selected.state} /></Descriptions.Item>
              <Descriptions.Item label="尝试次数">{selected.attempts}</Descriptions.Item>
              <Descriptions.Item label="外部请求 ID">{selected.external_request_id || '—'}</Descriptions.Item>
              <Descriptions.Item label="外部对象 ID">{selected.external_object_id || '—'}</Descriptions.Item>
              <Descriptions.Item label="下次投递">{new Date(selected.next_attempt_at).toLocaleString()}</Descriptions.Item>
            </Descriptions>
            <div>
              <Text type="secondary">Payload</Text>
              <PayloadViewer value={selected.payload_json} />
            </div>
            <div>
              <Text type="secondary">最后错误</Text>
              <PayloadViewer value={selected.last_error} />
            </div>
          </Space>
        )}
      </Drawer>

      <Modal
        title="确认人工补偿"
        open={!!compensating}
        onOk={confirmCompensate}
        onCancel={() => setCompensating(null)}
        okText="确认补偿"
        okButtonProps={{ danger: true }}
      >
        <p>
          将对 Outbox 记录 <Text code>{compensating?.id.slice(0, 8)}…</Text>
          ({compensating?.connector_code} / {compensating?.operation}) 触发补偿流程,
          状态将转为 <Tag color="warning">compensate_pending</Tag>。
        </p>
        <p style={{ color: '#888', fontSize: 12 }}>该操作会写入审计日志。</p>
      </Modal>
    </div>
  );
}
