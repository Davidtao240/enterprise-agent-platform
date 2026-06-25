import { useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { Alert, Button, Card, Descriptions, Input, Space, Spin, Typography, message } from 'antd';
import { approveTask, getApprovalTask, rejectTask } from '../services/api';
import { useAuthStore } from '../store/auth';

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
      .catch(() => message.error('Failed to load approval task'))
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
      message.success('Approved');
      navigate(`/workflows/${task.workflow_instance_id}`);
    } catch {
      message.error('Approval failed');
    } finally {
      setLoading(false);
    }
  };

  const handleReject = async () => {
    if (!id) return;
    if (!comment.trim()) {
      message.warning('Rejection requires a comment');
      return;
    }
    setLoading(true);
    try {
      await rejectTask(id, comment);
      message.success('Rejected');
      navigate(`/workflows/${task.workflow_instance_id}`);
    } catch {
      message.error('Rejection failed');
    } finally {
      setLoading(false);
    }
  };

  if (fetching) {
    return <Spin size="large" style={{ display: 'block', margin: '100px auto' }} />;
  }

  if (!task) {
    return <Alert type="error" message="Approval task not found" />;
  }

  return (
    <div style={{ maxWidth: 720, margin: '0 auto' }}>
      <Title level={4}>Approval Review</Title>
      <Card>
        <Descriptions column={1} size="small" style={{ marginBottom: 16 }}>
          <Descriptions.Item label="Task">{task.title}</Descriptions.Item>
          <Descriptions.Item label="Workflow">{task.workflow_title}</Descriptions.Item>
          <Descriptions.Item label="Status">{task.status}</Descriptions.Item>
        </Descriptions>

        {agentOutput && (
          <Card size="small" title="Report Summary" style={{ marginBottom: 16 }}>
            <p>{agentOutput.summary || 'No summary returned.'}</p>
            {agentOutput.warnings?.length > 0 && (
              <Alert
                type="warning"
                showIcon
                message="Warnings"
                description={agentOutput.warnings.map((w: any) => w.message || String(w)).join('\n')}
              />
            )}
          </Card>
        )}

        <TextArea
          rows={4}
          value={comment}
          onChange={(e) => setComment(e.target.value)}
          placeholder="Add your review comment..."
        />
        <div style={{ marginTop: 16 }}>
          <Space>
            <Button type="primary" onClick={handleApprove} loading={loading} disabled={task.status !== 'pending' || !canDecideApproval}>
              Approve
            </Button>
            <Button danger onClick={handleReject} loading={loading} disabled={task.status !== 'pending' || !canDecideApproval}>
              Reject
            </Button>
          </Space>
        </div>
      </Card>
    </div>
  );
}
