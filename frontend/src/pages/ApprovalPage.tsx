import { useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Button, Card, Input, Space, Typography, message } from 'antd';
import { approveTask, rejectTask } from '../services/api';

const { Title } = Typography;
const { TextArea } = Input;

export default function ApprovalPage() {
  const { id } = useParams<{ id: string }>();
  const [comment, setComment] = useState('');
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  const handleApprove = async () => {
    if (!id) return;
    setLoading(true);
    try {
      await approveTask(id, comment);
      message.success('Approved');
      navigate(-1);
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
      navigate(-1);
    } catch {
      message.error('Rejection failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{ maxWidth: 600, margin: '0 auto' }}>
      <Title level={4}>Approval Review</Title>
      <Card>
        {/* TODO: Phase 5 — Display report preview, agent summary, warnings */}
        <p>Report preview and agent analysis summary will be displayed here.</p>
        <div style={{ marginTop: 24 }}>
          <TextArea
            rows={4}
            value={comment}
            onChange={(e) => setComment(e.target.value)}
            placeholder="Add your review comment..."
          />
        </div>
        <div style={{ marginTop: 16 }}>
          <Space>
            <Button type="primary" onClick={handleApprove} loading={loading}>
              Approve
            </Button>
            <Button danger onClick={handleReject} loading={loading}>
              Reject
            </Button>
          </Space>
        </div>
      </Card>
    </div>
  );
}
