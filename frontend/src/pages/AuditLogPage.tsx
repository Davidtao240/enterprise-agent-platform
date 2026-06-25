import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Button, Descriptions, Drawer, Form, Input, Space, Table, Tag, Typography } from 'antd';
import { getAuditLogs } from '../services/api';

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
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<any>(null);

  const fetchLogs = (params: Record<string, string>) => {
    setLoading(true);
    getAuditLogs({ ...params, page_size: '50' })
      .then(({ data }) => setLogs(data.data))
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    const params = {
      trace_id: searchParams.get('trace_id') || '',
      business_app_code: searchParams.get('business_app_code') || '',
      action: searchParams.get('action') || '',
      actor_user_id: searchParams.get('actor_user_id') || '',
      resource_type: searchParams.get('resource_type') || '',
    };
    form.setFieldsValue(params);
    fetchLogs(Object.fromEntries(Object.entries(params).filter(([, value]) => value)));
  }, [queryKey, form]);

  const columns = [
    { title: 'Trace ID', dataIndex: 'trace_id', key: 'trace_id', ellipsis: true },
    { title: 'Action', dataIndex: 'action', key: 'action' },
    { title: 'Actor', dataIndex: 'actor_user_id', key: 'actor_user_id', ellipsis: true, render: (v: string) => v || '-' },
    { title: 'Business App', dataIndex: 'business_app_code', key: 'business_app_code', render: (v: string) => v || '-' },
    { title: 'Resource', key: 'resource', render: (_: any, r: any) => `${r.resource_type}:${r.resource_id}` },
    { title: 'Status', dataIndex: 'status', key: 'status', render: (status: string) => <Tag>{status}</Tag> },
    { title: 'Time', dataIndex: 'created_at', key: 'created_at' },
    {
      title: '',
      key: 'action_detail',
      render: (_: any, record: any) => <Button size="small" onClick={() => setSelected(record)}>Detail</Button>,
    },
  ];

  const applyFilters = (values: Record<string, string>) => {
    const params = Object.fromEntries(Object.entries(values).filter(([, value]) => value));
    setSearchParams(params);
  };

  return (
    <div>
      <Space style={{ marginBottom: 16, justifyContent: 'space-between', width: '100%' }}>
        <Title level={4} style={{ margin: 0 }}>Audit Logs</Title>
      </Space>

      <Form form={form} layout="inline" onFinish={applyFilters} style={{ marginBottom: 16 }}>
        <Form.Item name="trace_id">
          <Input placeholder="Trace ID" allowClear style={{ width: 260 }} />
        </Form.Item>
        <Form.Item name="business_app_code">
          <Input placeholder="Business App" allowClear style={{ width: 140 }} />
        </Form.Item>
        <Form.Item name="action">
          <Input placeholder="Action" allowClear style={{ width: 220 }} />
        </Form.Item>
        <Form.Item name="resource_type">
          <Input placeholder="Resource Type" allowClear style={{ width: 160 }} />
        </Form.Item>
        <Form.Item>
          <Space>
            <Button type="primary" htmlType="submit">Search</Button>
            <Button onClick={() => { form.resetFields(); setSearchParams({}); }}>Reset</Button>
          </Space>
        </Form.Item>
      </Form>

      <Table dataSource={logs} columns={columns} rowKey="id" loading={loading} scroll={{ x: 1200 }} />

      <Drawer title="Audit Log Detail" open={!!selected} onClose={() => setSelected(null)} width={640}>
        {selected && (
          <Space direction="vertical" style={{ width: '100%' }} size="middle">
            <Descriptions column={1} size="small" bordered>
              <Descriptions.Item label="Trace ID">{selected.trace_id}</Descriptions.Item>
              <Descriptions.Item label="Actor">{selected.actor_user_id || '-'}</Descriptions.Item>
              <Descriptions.Item label="Business App">{selected.business_app_code || '-'}</Descriptions.Item>
              <Descriptions.Item label="Action">{selected.action}</Descriptions.Item>
              <Descriptions.Item label="Resource Type">{selected.resource_type}</Descriptions.Item>
              <Descriptions.Item label="Resource ID">{selected.resource_id}</Descriptions.Item>
              <Descriptions.Item label="Status">{selected.status}</Descriptions.Item>
              <Descriptions.Item label="Created At">{selected.created_at}</Descriptions.Item>
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
