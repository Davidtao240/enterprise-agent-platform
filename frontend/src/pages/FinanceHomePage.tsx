import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Button, Form, Input, Modal, Space, Table, Tag, Typography, Upload } from 'antd';
import { PlusOutlined, UploadOutlined } from '@ant-design/icons';
import { getWorkflowInstances, createWorkflowInstance, startWorkflow, uploadFile } from '../services/api';
import { useAuthStore } from '../store/auth';
import { tStatus } from '../utils/i18n';

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
  const hasPermission = useAuthStore((s) => s.hasPermission);
  const canCreateTask = hasPermission('workflow:create') && hasPermission('file:upload') && hasPermission('workflow:start');

  const fetchInstances = () => {
    setLoading(true);
    getWorkflowInstances({ business_app_code: 'finance' })
      .then(({ data }) => setInstances(data.data))
      .finally(() => setLoading(false));
  };

  useEffect(() => { fetchInstances(); }, []);

  // ── Polling: 列表中存在活跃实例时每 5 秒刷新 ──
  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null);
  useEffect(() => {
    const hasActive = instances.some((i) => ['running', 'waiting_review'].includes(i.status));
    if (!hasActive) {
      if (pollingRef.current) clearInterval(pollingRef.current);
      pollingRef.current = null;
      return;
    }
    pollingRef.current = setInterval(() => {
      getWorkflowInstances({ business_app_code: 'finance' })
        .then(({ data }) => setInstances(data.data));
    }, 5000);
    return () => {
      if (pollingRef.current) clearInterval(pollingRef.current);
    };
  }, [instances]);

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
    { title: '标题', dataIndex: 'title', key: 'title' },
    {
      title: '状态', dataIndex: 'status', key: 'status',
      render: (s: string) => <Tag color={statusColor[s]}>{tStatus(s)}</Tag>,
    },
    { title: '创建时间', dataIndex: 'created_at', key: 'created_at' },
    {
      title: '操作', key: 'action',
      render: (_: any, record: any) => (
        <Button size="small" onClick={() => navigate(`/workflows/${record.id}`)}>详情</Button>
      ),
    },
  ];

  return (
    <div>
      <Space style={{ marginBottom: 16, justifyContent: 'space-between', width: '100%' }}>
        <Title level={4} style={{ margin: 0 }}>财务运营报告</Title>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalOpen(true)} disabled={!canCreateTask}>
          新建报告任务
        </Button>
      </Space>

      <Table dataSource={instances} columns={columns} rowKey="id" loading={loading} />

      <Modal title="新建运营报告任务" open={modalOpen} onCancel={() => setModalOpen(false)} onOk={() => form.submit()} okText="创建并启动" cancelText="取消">
        <Form form={form} layout="vertical" onFinish={handleCreate}>
          <Form.Item name="title" label="标题" rules={[{ required: true, message: '请输入任务标题' }]}>
            <Input placeholder="例如：2026-05 财务运营数据报告" />
          </Form.Item>
          <Form.Item name="month" label="期间" rules={[{ required: true, message: '请输入报告期间' }]}>
            <Input placeholder="YYYY-MM" />
          </Form.Item>
          <Form.Item name="department" label="部门" rules={[{ required: true, message: '请输入部门' }]}>
            <Input placeholder="财务中心" />
          </Form.Item>
          <Form.Item name="file" label="上传数据文件（CSV/Excel）" rules={[{ required: true, message: '请选择数据文件' }]}>
            <Upload accept=".csv,.xlsx" maxCount={1} beforeUpload={() => false}>
              <Button icon={<UploadOutlined />}>选择文件</Button>
            </Upload>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
