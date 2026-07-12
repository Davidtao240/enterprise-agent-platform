import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Button, Col, DatePicker, Descriptions, Drawer, Form, Input, Row, Space, Statistic, Table, Tag, Typography } from 'antd';
import dayjs from 'dayjs';
import { getAuditLogs, getAuditStats } from '../services/api';
import { tStatus } from '../utils/i18n';

const { Title } = Typography;

function formatDetail(raw: unknown): string {
  if (!raw) return '{}';
  if (typeof raw !== 'string') return JSON.stringify(raw, null, 2);
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}

export default function AuditLogPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const queryKey = searchParams.toString();
  const [form] = Form.useForm();
  const [logs, setLogs] = useState<any[]>([]);
  const [stats, setStats] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<any>(null);

  const fetchLogs = (params: Record<string, string>) => {
    setLoading(true);
    Promise.all([
      getAuditLogs({ ...params, page_size: '50' }),
      getAuditStats(params),
    ])
      .then(([logsRes, statsRes]) => {
        setLogs(logsRes.data.data);
        setStats(statsRes.data.data);
      })
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    const params = {
      trace_id: searchParams.get('trace_id') || '',
      business_app_code: searchParams.get('business_app_code') || '',
      action: searchParams.get('action') || '',
      actor_user_id: searchParams.get('actor_user_id') || '',
      resource_type: searchParams.get('resource_type') || '',
      resource_id: searchParams.get('resource_id') || '',
      status: searchParams.get('status') || '',
      created_from: searchParams.get('created_from') || '',
      created_to: searchParams.get('created_to') || '',
    };
    form.setFieldsValue({
      ...params,
      created_range: params.created_from && params.created_to
        ? [dayjs(params.created_from), dayjs(params.created_to)]
        : undefined,
    });
    fetchLogs(Object.fromEntries(Object.entries(params).filter(([, value]) => value)));
  }, [queryKey, form]);

  const columns = [
    { title: '追踪 ID', dataIndex: 'trace_id', key: 'trace_id', ellipsis: true },
    { title: '动作', dataIndex: 'action', key: 'action' },
    { title: '操作人', dataIndex: 'actor_user_id', key: 'actor_user_id', ellipsis: true, render: (v: string) => v || '-' },
    { title: '业务应用', dataIndex: 'business_app_code', key: 'business_app_code', render: (v: string) => v || '-' },
    { title: '资源', key: 'resource', render: (_: any, r: any) => `${r.resource_type}:${r.resource_id}` },
    { title: '状态', dataIndex: 'status', key: 'status', render: (status: string) => <Tag>{tStatus(status)}</Tag> },
    { title: '时间', dataIndex: 'created_at', key: 'created_at' },
    {
      title: '',
      key: 'action_detail',
      render: (_: any, record: any) => <Button size="small" onClick={() => setSelected(record)}>详情</Button>,
    },
  ];

  const applyFilters = (values: Record<string, any>) => {
    const nextValues = { ...values };
    if (values.created_range?.length === 2) {
      nextValues.created_from = values.created_range[0].startOf('day').toISOString();
      nextValues.created_to = values.created_range[1].endOf('day').toISOString();
    }
    delete nextValues.created_range;
    const params = Object.fromEntries(Object.entries(nextValues).filter(([, value]) => value));
    setSearchParams(params);
  };

  return (
    <div>
      <Space style={{ marginBottom: 16, justifyContent: 'space-between', width: '100%' }}>
        <Title level={4} style={{ margin: 0 }}>审计日志</Title>
      </Space>

      <Form form={form} layout="inline" onFinish={applyFilters} style={{ marginBottom: 16 }}>
        <Form.Item name="trace_id">
          <Input placeholder="追踪 ID" allowClear style={{ width: 260 }} />
        </Form.Item>
        <Form.Item name="business_app_code">
          <Input placeholder="业务应用" allowClear style={{ width: 140 }} />
        </Form.Item>
        <Form.Item name="action">
          <Input placeholder="动作" allowClear style={{ width: 220 }} />
        </Form.Item>
        <Form.Item name="resource_type">
          <Input placeholder="资源类型" allowClear style={{ width: 160 }} />
        </Form.Item>
        <Form.Item name="resource_id">
          <Input placeholder="资源 ID" allowClear style={{ width: 220 }} />
        </Form.Item>
        <Form.Item name="status">
          <Input placeholder="状态" allowClear style={{ width: 140 }} />
        </Form.Item>
        <Form.Item name="actor_user_id">
          <Input placeholder="操作人用户 ID" allowClear style={{ width: 220 }} />
        </Form.Item>
        <Form.Item name="created_range">
          <DatePicker.RangePicker allowClear />
        </Form.Item>
        <Form.Item>
          <Space>
            <Button type="primary" htmlType="submit">搜索</Button>
            <Button onClick={() => { form.resetFields(); setSearchParams({}); }}>重置</Button>
          </Space>
        </Form.Item>
      </Form>

      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={24} md={6}>
          <Statistic title="日志总数" value={stats?.total || 0} />
        </Col>
        <Col xs={24} md={9}>
          <Space direction="vertical" size={4}>
            <Typography.Text type="secondary">按状态统计</Typography.Text>
            <Space wrap>
              {(stats?.by_status || []).map((item: any) => <Tag key={item.key}>{tStatus(item.key)}: {item.count}</Tag>)}
            </Space>
          </Space>
        </Col>
        <Col xs={24} md={9}>
          <Space direction="vertical" size={4}>
            <Typography.Text type="secondary">高频动作</Typography.Text>
            <Space wrap>
              {(stats?.by_action || []).slice(0, 6).map((item: any) => <Tag key={item.key}>{item.key}: {item.count}</Tag>)}
            </Space>
          </Space>
        </Col>
      </Row>

      <Table dataSource={logs} columns={columns} rowKey="id" loading={loading} scroll={{ x: 1200 }} locale={{ emptyText: '暂无匹配的审计日志' }} />

      <Drawer title="审计日志详情" open={!!selected} onClose={() => setSelected(null)} width={640}>
        {selected && (
          <Space direction="vertical" style={{ width: '100%' }} size="middle">
            <Descriptions column={1} size="small" bordered>
              <Descriptions.Item label="追踪 ID">{selected.trace_id}</Descriptions.Item>
              <Descriptions.Item label="操作人">{selected.actor_user_id || '-'}</Descriptions.Item>
              <Descriptions.Item label="业务应用">{selected.business_app_code || '-'}</Descriptions.Item>
              <Descriptions.Item label="动作">{selected.action}</Descriptions.Item>
              <Descriptions.Item label="资源类型">{selected.resource_type}</Descriptions.Item>
              <Descriptions.Item label="资源 ID">{selected.resource_id}</Descriptions.Item>
              <Descriptions.Item label="状态">{tStatus(selected.status)}</Descriptions.Item>
              <Descriptions.Item label="创建时间">{selected.created_at}</Descriptions.Item>
            </Descriptions>
            <pre style={{ whiteSpace: 'pre-wrap', background: '#f5f5f5', padding: 12, margin: 0 }}>
              {formatDetail(selected.detail_json)}
            </pre>
          </Space>
        )}
      </Drawer>
    </div>
  );
}
