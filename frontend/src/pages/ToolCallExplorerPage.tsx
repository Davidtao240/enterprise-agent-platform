import { useCallback, useEffect, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  Button, Card, Descriptions, Drawer, Input, Select, Space, Table, Tag, Typography,
} from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { getOpsToolCalls } from '../services/api';
import StatusBadge from '../components/StatusBadge';
import PayloadViewer from '../components/PayloadViewer';

const { Title } = Typography;

// ToolCall 模型(M2 契约,精简展示字段)。
interface ToolCall {
  id: string;
  run_id: string;
  tool_id: string;
  tool_version: string;
  business_app_code: string;
  risk_level: string;
  status: string;
  idempotency_key: string;
  input_summary_json?: string;
  output_summary_json?: string;
  error_json?: string;
  verification_json?: string;
  trace_id?: string;
  is_dead_letter: boolean;
  retry_count: number;
  created_at: string;
  updated_at: string;
}

const STATUS_OPTIONS = [
  'requested', 'pending_approval', 'executing', 'succeeded',
  'failed', 'indeterminate', 'cancelled',
].map((s) => ({ value: s, label: s }));

export default function ToolCallExplorerPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const initialStatus = (location.state as { status?: string } | null)?.status || '';

  const [items, setItems] = useState<ToolCall[]>([]);
  const [loading, setLoading] = useState(true);
  const [status, setStatus] = useState(initialStatus);
  const [toolId, setToolId] = useState('');
  const [selected, setSelected] = useState<ToolCall | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const params: Record<string, string> = { limit: '50' };
      if (status) params.status = status;
      if (toolId.trim()) params.tool_id = toolId.trim();
      const { data } = await getOpsToolCalls(params);
      setItems(data.data.items || []);
    } finally {
      setLoading(false);
    }
  }, [status, toolId]);

  useEffect(() => {
    load();
  }, [load]);

  const columns = [
    {
      title: 'Tool Call ID',
      dataIndex: 'id',
      width: 150,
      render: (v: string) => <Typography.Text copyable style={{ fontSize: 12 }}>{v.slice(0, 8)}…</Typography.Text>,
    },
    { title: 'Tool', dataIndex: 'tool_id', width: 140 },
    { title: '状态', dataIndex: 'status', width: 130, render: (v: string) => <StatusBadge status={v} /> },
    { title: '风险', dataIndex: 'risk_level', width: 90, render: (v: string) => <Tag color={v === 'high' ? 'red' : v === 'medium' ? 'orange' : 'green'}>{v}</Tag> },
    {
      title: 'Run',
      dataIndex: 'run_id',
      width: 110,
      render: (v: string) => (
        <a onClick={() => navigate(`/runs/${v}`)}>{v.slice(0, 8)}…</a>
      ),
    },
    { title: '重试', dataIndex: 'retry_count', width: 70 },
    { title: '死信', dataIndex: 'is_dead_letter', width: 70, render: (v: boolean) => (v ? <Tag color="red">DLQ</Tag> : '—') },
    { title: '更新时间', dataIndex: 'updated_at', width: 170, render: (v: string) => new Date(v).toLocaleString() },
    { title: '操作', width: 80, render: (_: unknown, row: ToolCall) => <Button size="small" onClick={() => setSelected(row)}>详情</Button> },
  ];

  return (
    <div>
      <Title level={4}>Tool Call 探索器</Title>
      <Card size="small">
        <Space wrap style={{ marginBottom: 12 }}>
          <Select
            allowClear
            placeholder="状态过滤"
            style={{ width: 180 }}
            options={STATUS_OPTIONS}
            value={status || undefined}
            onChange={(v) => setStatus(v || '')}
          />
          <Input.Search
            placeholder="按 Tool ID 过滤"
            style={{ width: 240 }}
            allowClear
            onSearch={(v) => setToolId(v)}
          />
          <Button icon={<ReloadOutlined />} onClick={load}>刷新</Button>
        </Space>
        <Table
          rowKey="id"
          size="small"
          loading={loading}
          dataSource={items}
          columns={columns}
          pagination={{ pageSize: 20, showSizeChanger: false }}
          scroll={{ x: 1100 }}
        />
      </Card>

      <Drawer
        title="Tool Call 详情"
        width={520}
        open={!!selected}
        onClose={() => setSelected(null)}
      >
        {selected && (
          <Space direction="vertical" style={{ width: '100%' }} size="middle">
            <Descriptions size="small" column={1} bordered>
              <Descriptions.Item label="ID"><Typography.Text copyable style={{ fontSize: 11 }}>{selected.id}</Typography.Text></Descriptions.Item>
              <Descriptions.Item label="Tool">{selected.tool_id} @ {selected.tool_version}</Descriptions.Item>
              <Descriptions.Item label="状态"><StatusBadge status={selected.status} /></Descriptions.Item>
              <Descriptions.Item label="业务域">{selected.business_app_code}</Descriptions.Item>
              <Descriptions.Item label="Idempotency Key">{selected.idempotency_key}</Descriptions.Item>
              <Descriptions.Item label="Trace ID"><Typography.Text copyable style={{ fontSize: 11 }}>{selected.trace_id || '—'}</Typography.Text></Descriptions.Item>
            </Descriptions>
            <div>
              <Typography.Text type="secondary">Input Summary</Typography.Text>
              <PayloadViewer value={selected.input_summary_json} />
            </div>
            <div>
              <Typography.Text type="secondary">Output Summary</Typography.Text>
              <PayloadViewer value={selected.output_summary_json} />
            </div>
            <div>
              <Typography.Text type="secondary">Verification</Typography.Text>
              <PayloadViewer value={selected.verification_json} />
            </div>
            <div>
              <Typography.Text type="secondary">Error</Typography.Text>
              <PayloadViewer value={selected.error_json} />
            </div>
          </Space>
        )}
      </Drawer>
    </div>
  );
}
