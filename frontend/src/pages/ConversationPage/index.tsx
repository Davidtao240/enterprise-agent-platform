import { useCallback, useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Alert, Spin, Typography } from 'antd';
import ChatWindow from '../../components/ChatWindow';
import { getConversation, Conversation } from '../../services/conversation';

const { Title } = Typography;

export default function ConversationPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();

  const [loading, setLoading] = useState(true);
  const [conversation, setConversation] = useState<Conversation | null>(null);
  const [error, setError] = useState<string | null>(null);

  const loadConversation = useCallback(async () => {
    if (!id) return;
    try {
      const data = await getConversation(id);
      setConversation(data.conversation ?? data);
    } catch (e: unknown) {
      const message = e instanceof Error ? e.message : '加载失败';
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    setLoading(true);
    setError(null);
    loadConversation();
  }, [loadConversation]);

  if (loading) {
    return <Spin size="large" style={{ display: 'block', margin: '100px auto' }} />;
  }

  if (error || !conversation) {
    return (
      <div>
        <Title level={4}>对话</Title>
        <Alert
          type="error"
          message={error || '对话不存在或无权访问'}
          showIcon
          action={
            <button onClick={() => navigate('/')}>返回首页</button>
          }
        />
      </div>
    );
  }

  return (
    <div style={{ height: 'calc(100vh - 48px - 48px)', margin: -24, overflow: 'hidden' }}>
      <ChatWindow
        conversationId={conversation.id}
        agentPackageCode={conversation.agent_package_code}
      />
    </div>
  );
}