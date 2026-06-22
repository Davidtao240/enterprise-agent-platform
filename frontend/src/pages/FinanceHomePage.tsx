import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Button, Form, Input, Modal, Space, Table, Tag, Typography, Upload } from 'antd';
import { PlusOutlined, UploadOutlined } from '@ant-design/icons';
import { getWorkflowInstances, createWorkflowInstance, startWorkflow, uploadFile } from '../services/api';

const { Title } = Typography;

const statusColor: Record<string, string> = {
  draft: 'default',
  running: 'processing',
  waiting_review: 'warning',
  approved: 'success',
  rejected: 'error',
  archived: 'blue',
  failed: 'red',
  cancelled: 'default',
};

export default function FinanceHomePage() {
  const [instances, setInstances] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [modalOpen, setModalOpen] = useState(false);
  const [form] = Form.useForm();
  const navigate = useNavigate();

  const fetchInstances = () => {
    setLoading(true);
    getWorkflowInstances({ business_app_code: 'finance' })
      .then(({ data }) => setInstances(data.data))
      .finally(() => setLoading(false));
  };

  useEffect(() => { fetchInstances(); }, []);

  const handleCreate = async (values: any) => {
    let fileId: string | undefined;

    if (values.file) {
      const formData = new FormData();
      formData.append('business_app_code', 'finance');
      formData.append('file_role', 'source');
      formData.append('file', values.file.file.originFileObj);
      const uploadRes = await uploadFile(formData);
      fileId = uploadRes.data.data.file_id;
    }

    const { data } = await createWorkflowInstance({
      business_app_code: 'finance',
      workflow_template_key: 'finance_operating_report',
      title: values.title,
      input: { month: values.month, department: values.department, file_id: fileId },
    });

    await startWorkflow(data.data.id);
    setModalOpen(false);
    form.resetFields();
    fetchInstances();
  };

  const columns = [
    { title: 'Title', dataIndex: 'title', key: 'title' },
    {
      title: 'Status', dataIndex: 'status', key: 'status',
      render: (s: string) => <Tag color={statusColor[s]}>{s}</Tag>,
    },
    { title: 'Created', dataIndex: 'created_at', key: 'created_at' },
    {
      title: '', key: 'action',
      render: (_: any, record: any) => (
        <Button size="small" onClick={() => navigate(`/workflows/${record.id}`)}>Detail</Button>
      ),
    },
  ];

  return (
    <div>
      <Space style={{ marginBottom: 16, justifyContent: 'space-between', width: '100%' }}>
        <Title level={4} style={{ margin: 0 }}>Finance Operating Reports</Title>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalOpen(true)}>
          New Report Task
        </Button>
      </Space>

      <Table dataSource={instances} columns={columns} rowKey="id" loading={loading} />

      <Modal title="New Operating Report Task" open={modalOpen} onCancel={() => setModalOpen(false)} onOk={() => form.submit()}>
        <Form form={form} layout="vertical" onFinish={handleCreate}>
          <Form.Item name="title" label="Title" rules={[{ required: true }]}>
            <Input placeholder="e.g., 2026-05 Operating Data Report" />
          </Form.Item>
          <Form.Item name="month" label="Period" rules={[{ required: true }]}>
            <Input placeholder="YYYY-MM" />
          </Form.Item>
          <Form.Item name="department" label="Department" rules={[{ required: true }]}>
            <Input placeholder="Finance Center" />
          </Form.Item>
          <Form.Item name="file" label="Upload Data (CSV/Excel)" rules={[{ required: true }]}>
            <Upload accept=".csv,.xlsx" maxCount={1} beforeUpload={() => false}>
              <Button icon={<UploadOutlined />}>Select File</Button>
            </Upload>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
