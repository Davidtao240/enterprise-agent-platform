import { useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { Alert, Button, Card, Descriptions, Input, Space, Spin, Typography, message } from 'antd';
import { DownloadOutlined, FileExcelOutlined } from '@ant-design/icons';
import {
  approveTask,
  downloadFile,
  getApprovalTask,
  getFile,
  getWorkflowInstance,
  rejectTask,
} from '../services/api';
import { useAuthStore } from '../store/auth';
import {
  localizeApprovalTitle,
  localizeFinanceText,
  tStatus,
} from '../utils/i18n';

const { Title } = Typography;
const { TextArea } = Input;

function parseJSON(raw: unknown): Record<string, any> {
  if (!raw) return {};
  if (typeof raw === 'object') return raw as Record<string, any>;
  try {
    return JSON.parse(String(raw));
  } catch {
    return {};
  }
}

function formatFileSize(size?: number): string {
  if (size == null) return '-';
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

export default function ApprovalPage() {
  const { id } = useParams<{ id: string }>();
  const [task, setTask] = useState<any>(null);
  const [attachment, setAttachment] = useState<any>(null);
  const [attachmentError, setAttachmentError] = useState('');
  const [comment, setComment] = useState('');
  const [loading, setLoading] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [fetching, setFetching] = useState(true);
  const navigate = useNavigate();
  const canDecideApproval = useAuthStore((s) => s.hasPermission('approval:decide'));

  useEffect(() => {
    if (!id) return;
    getApprovalTask(id)
      .then(async ({ data }) => {
        const approvalTask = data.data;
        setTask(approvalTask);
        try {
          const workflowRes = await getWorkflowInstance(approvalTask.workflow_instance_id);
          const workflowInput = parseJSON(workflowRes.data.data.input_json);
          if (!workflowInput.file_id) {
            setAttachmentError('该流程未关联原始附件。');
            return;
          }
          const fileRes = await getFile(String(workflowInput.file_id));
          setAttachment(fileRes.data.data);
        } catch {
          setAttachmentError('原始附件信息加载失败，请联系流程发起人核验。');
        }
      })
      .catch(() => message.error('审批任务加载失败'))
      .finally(() => setFetching(false));
  }, [id]);

  const agentOutput = useMemo(() => {
    if (!task?.agent_output_json) return null;
    try {
      return JSON.parse(task.agent_output_json);
    } catch {
      return null;
    }
  }, [task]);

  const localizeAgentText = (value?: string | null) =>
    task?.business_app_code === 'finance' ? localizeFinanceText(value) : (value || '');

  const handleDownload = async () => {
    if (!attachment) return;
    setDownloading(true);
    try {
      const identifier = attachment.storage_key || attachment.id;
      const response = await downloadFile(identifier);
      const url = URL.createObjectURL(response.data);
      const link = document.createElement('a');
      link.href = url;
      link.download = attachment.original_filename || '财务报表';
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
    } catch {
      message.error('附件下载失败');
    } finally {
      setDownloading(false);
    }
  };

  const handleApprove = async () => {
    if (!id) return;
    setLoading(true);
    try {
      await approveTask(id, comment);
      message.success('已审批通过');
      navigate(`/workflows/${task.workflow_instance_id}`);
    } catch {
      message.error('审批通过失败');
    } finally {
      setLoading(false);
    }
  };

  const handleReject = async () => {
    if (!id) return;
    if (!comment.trim()) {
      message.warning('拒绝时必须填写审批意见');
      return;
    }
    setLoading(true);
    try {
      await rejectTask(id, comment);
      message.success('已拒绝');
      navigate(`/workflows/${task.workflow_instance_id}`);
    } catch {
      message.error('拒绝失败');
    } finally {
      setLoading(false);
    }
  };

  if (fetching) {
    return <Spin size="large" style={{ display: 'block', margin: '100px auto' }} />;
  }

  if (!task) {
    return <Alert type="error" message="未找到审批任务" />;
  }

  return (
    <div style={{ maxWidth: 720, margin: '0 auto' }}>
      <Title level={4}>审批复核</Title>
      <Card>
        <Descriptions column={1} size="small" style={{ marginBottom: 16 }}>
          <Descriptions.Item label="任务">{localizeApprovalTitle(task.title)}</Descriptions.Item>
          <Descriptions.Item label="流程">{task.workflow_title}</Descriptions.Item>
          <Descriptions.Item label="状态">{tStatus(task.status)}</Descriptions.Item>
        </Descriptions>

        <Card size="small" title="原始附件" style={{ marginBottom: 16 }}>
          {attachment ? (
            <Space style={{ width: '100%', justifyContent: 'space-between' }}>
              <Space>
                <FileExcelOutlined style={{ color: '#389e0d', fontSize: 20 }} />
                <div>
                  <Typography.Text strong>{attachment.original_filename}</Typography.Text>
                  <br />
                  <Typography.Text type="secondary">
                    {formatFileSize(attachment.size_bytes)} · {attachment.content_type || '未知文件类型'}
                  </Typography.Text>
                </div>
              </Space>
              <Button
                icon={<DownloadOutlined />}
                onClick={handleDownload}
                loading={downloading}
              >
                下载查看
              </Button>
            </Space>
          ) : (
            <Alert
              type={attachmentError ? 'warning' : 'info'}
              showIcon
              message={attachmentError || '正在加载原始附件...'}
            />
          )}
        </Card>

        {agentOutput && (
          <Card size="small" title="报告摘要" style={{ marginBottom: 16 }}>
            <p>{localizeAgentText(agentOutput.summary) || '暂无摘要。'}</p>
            {agentOutput.warnings?.length > 0 && (
              <Alert
                type="warning"
                showIcon
                message="风险提示"
                description={agentOutput.warnings
                  .map((w: any) => localizeAgentText(w.message || String(w)))
                  .join('\n')}
              />
            )}
          </Card>
        )}

        <TextArea
          rows={4}
          value={comment}
          onChange={(e) => setComment(e.target.value)}
          placeholder="请输入审批意见..."
        />
        <div style={{ marginTop: 16 }}>
          <Space>
            <Button type="primary" onClick={handleApprove} loading={loading} disabled={task.status !== 'pending' || !canDecideApproval}>
              通过
            </Button>
            <Button danger onClick={handleReject} loading={loading} disabled={task.status !== 'pending' || !canDecideApproval}>
              拒绝
            </Button>
          </Space>
        </div>
      </Card>
    </div>
  );
}
