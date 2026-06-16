import { useEffect, useState } from 'react';
import { Table, Typography } from 'antd';
import { getAuditLogs } from '../services/api';

const { Title } = Typography;

export default function AuditLogPage() {
  const [logs, setLogs] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    getAuditLogs({})
      .then(({ data }) => setLogs(data.data))
      .finally(() => setLoading(false));
  }, []);

  const columns = [
    { title: 'Action', dataIndex: 'action', key: 'action' },
    { title: 'Resource', dataIndex: 'resource_type', key: 'resource_type' },
    { title: 'Status', dataIndex: 'status', key: 'status' },
    { title: 'Time', dataIndex: 'created_at', key: 'created_at' },
  ];

  return (
    <div>
      <Title level={4}>Audit Logs</Title>
      <Table dataSource={logs} columns={columns} rowKey="id" loading={loading} />
    </div>
  );
}
