import { useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { Alert, Button, Card, Descriptions, Input, Space, Spin, Typography, message } from 'antd';
import { approveTask, getApprovalTask, rejectTask } from '../services/api';
import { useAuthStore } from '../store/auth';
import { tStatus } from '../utils/i18n';

const { Title } = Typography;
const { TextArea } = Input;

export default function ApprovalPage() {
  const { id } = useParams<{ id: string }>();
  const [task, setTask] = useState<any>(null);
  const [comment, setComment] = useState('');
  const [loading, setLoading] = useState(false);
  const [fetching, setFetching] = useState(true);
  const navigate = useNavigate();
  const canDecideApproval = useAuthStore((s) => s.hasPermission('approval:decide'));

  useEffect(() => {
    if (!id) return;
    getApprovalTask(id)
      .then(({ data }) => setTask(data.data))
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
          <Descriptions.Item label="任务">{task.title}</Descriptions.Item>
          <Descriptions.Item label="流程">{task.workflow_title}</Descriptions.Item>
          <Descriptions.Item label="状态">{tStatus(task.status)}</Descriptions.Item>
        </Descriptions>

        {agentOutput && (
          <Card size="small" title="报告摘要" style={{ marginBottom: 16 }}>
            <p>{agentOutput.summary || '暂无摘要。'}</p>
            {agentOutput.warnings?.length > 0 && (
              <Alert
                type="warning"
                showIcon
                message="风险提示"
                description={agentOutput.warnings.map((w: any) => w.message || String(w)).join('\n')}
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
