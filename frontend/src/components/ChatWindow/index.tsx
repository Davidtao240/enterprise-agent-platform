import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Layout, Input, Button, List, Typography, Spin, Empty, Space, Tag, Badge, message,
} from 'antd';
import { SendOutlined, PlusOutlined, StopOutlined } from '@ant-design/icons';
import MessageBubble from '../MessageBubble';
import ClarificationCard from '../ClarificationCard';
import {
  Conversation, ConversationMessage, ClarificationRequest, ApprovalRequest,
  listConversations, sendMessage as sendMessageApi,
  answerClarification, cancelRun, createConversation,
} from '../../services/conversation';
import { createSSEConnection, SSEConnection } from '../../services/sse';

const { Content, Sider } = Layout;
const { Text } = Typography;
const { TextArea } = Input;

interface ChatWindowProps {
  conversationId: string;
  agentPackageCode: string;
}

interface MessageItem extends ConversationMessage {
  isStreaming?: boolean;
}

export default function ChatWindow({ conversationId, agentPackageCode }: ChatWindowProps) {
  const navigate = useNavigate();
  const sseRef = useRef<SSEConnection | null>(null);
  const messageListRef = useRef<HTMLDivElement>(null);

  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [loadingList, setLoadingList] = useState(true);
  const [messages, setMessages] = useState<MessageItem[]>([]);
  const [inputText, setInputText] = useState('');
  const [sending, setSending] = useState(false);
  const [activeRun, setActiveRun] = useState(false);
  const [clarification, setClarification] = useState<ClarificationRequest | null>(null);
  const [approval, setApproval] = useState<ApprovalRequest | null>(null);
  const [streamingMessageId, setStreamingMessageId] = useState<string | null>(null);
  const [connected, setConnected] = useState(false);
  const [lastEventId, setLastEventId] = useState<string | undefined>(undefined);

  const scrollToBottom = useCallback(() => {
    if (messageListRef.current) {
      messageListRef.current.scrollTop = messageListRef.current.scrollHeight;
    }
  }, []);

  useEffect(() => {
    scrollToBottom();
  }, [messages, scrollToBottom]);

  const loadConversationList = useCallback(async () => {
    try {
      const { data } = await listConversations('agent_package_code');
      setConversations(data.data || []);
    } catch {
      setConversations([]);
    } finally {
      setLoadingList(false);
    }
  }, []);

  useEffect(() => {
    loadConversationList();
  }, [loadConversationList]);

  const handleNewConversation = async () => {
    try {
      const { data } = await createConversation(agentPackageCode);
      message.success('新对话已创建');
      navigate(`/conversations/${data.data.id}`);
    } catch {
      message.error('创建对话失败');
    }
  };

  useEffect(() => {
    if (!conversationId) return;

    setMessages([]);
    setClarification(null);
    setApproval(null);
    setActiveRun(false);
    setStreamingMessageId(null);

    const sse = createSSEConnection(conversationId, lastEventId);
    sseRef.current = sse;
    setConnected(true);

    const unsubscribe = sse.on((event) => {
      if (event.eventId) setLastEventId(event.eventId);

      switch (event.type) {
        case 'conversation.started':
        case 'run.started':
          setActiveRun(true);
          break;

        case 'message.delta': {
          const { message_id, delta, content } = event.data as {
            message_id?: string; delta?: string; content?: string;
          };
          if (message_id && delta) {
            setStreamingMessageId(message_id);
            setMessages((prev) => {
              const existing = prev.find((m) => m.id === message_id);
              if (existing) {
                return prev.map((m) =>
                  m.id === message_id
                    ? { ...m, content: m.content + delta }
                    : m
                );
              }
              return [
                ...prev,
                {
                  id: message_id,
                  role: 'assistant',
                  content: delta,
                  isStreaming: true,
                  created_at: new Date().toISOString(),
                },
              ];
            });
          } else if (message_id && content) {
            setMessages((prev) => {
              const existing = prev.find((m) => m.id === message_id);
              if (existing) {
                return prev.map((m) =>
                  m.id === message_id
                    ? { ...m, content: content }
                    : m
                );
              }
              return [
                ...prev,
                {
                  id: message_id,
                  role: 'assistant',
                  content: content,
                  isStreaming: true,
                  created_at: new Date().toISOString(),
                },
              ];
            });
          }
          break;
        }

        case 'message.completed': {
          const { message_id, tokens, content } = event.data as {
            message_id: string; tokens?: number; content?: string;
          };
          setStreamingMessageId(null);
          setMessages((prev) =>
            prev.map((m) =>
              m.id === message_id
                ? { ...m, isStreaming: false, tokens, content: content ?? m.content }
                : m
            )
          );
          break;
        }

        case 'clarification.requested': {
          const interrupt = event.data as unknown as ClarificationRequest;
          setClarification(interrupt);
          setActiveRun(false);
          break;
        }

        case 'approval.requested': {
          const req = event.data as unknown as ApprovalRequest;
          setApproval(req);
          setActiveRun(false);
          break;
        }

        case 'run.completed':
        case 'done':
          setActiveRun(false);
          setStreamingMessageId(null);
          break;

        case 'run.failed':
          setActiveRun(false);
          setStreamingMessageId(null);
          break;

        case 'error':
          if ((event.data as { message?: string }).message) {
            message.error((event.data as { message: string }).message);
          }
          break;

        default:
          break;
      }
    });

    return () => {
      unsubscribe();
      sse.close();
      sseRef.current = null;
      setConnected(false);
    };
  }, [conversationId]);

  const handleSend = async () => {
    const text = inputText.trim();
    if (!text || activeRun || sending) return;

    const userMessage: MessageItem = {
      id: `local-${Date.now()}`,
      role: 'user',
      content: text,
      created_at: new Date().toISOString(),
    };
    setMessages((prev) => [...prev, userMessage]);
    setInputText('');
    setSending(true);

    try {
      await sendMessageApi(conversationId, text);
      setActiveRun(true);
    } catch {
      message.error('发送失败，请重试');
      setMessages((prev) => prev.slice(0, -1));
    } finally {
      setSending(false);
    }
  };

  const handleCancelRun = async () => {
    try {
      await cancelRun(conversationId);
      message.success('已请求取消');
      setActiveRun(false);
    } catch {
      message.error('取消失败');
    }
  };

  const handleClarificationSubmit = async (answer: Record<string, unknown>) => {
    if (!clarification) return;
    await answerClarification(conversationId, clarification.id, answer);
    setClarification(null);
    setActiveRun(true);
  };

  const handleClarificationCancel = () => {
    setClarification(null);
    message.info('已取消补充信息');
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  const groupedConversations = () => {
    const groups: Record<string, Conversation[]> = {};
    conversations.forEach((c) => {
      const key = c.agent_package_code || '其他';
      if (!groups[key]) groups[key] = [];
      groups[key].push(c);
    });
    return groups;
  };

  return (
    <Layout style={{ height: 'calc(100vh - 48px)' }}>
      <Sider
        width={240}
        style={{
          background: '#fff',
          borderRight: '1px solid #f0f0f0',
          padding: '12px 8px',
          overflow: 'auto',
        }}
      >
        <Button
          block
          icon={<PlusOutlined />}
          onClick={handleNewConversation}
          style={{ marginBottom: 12 }}
        >
          新建对话
        </Button>

        {loadingList ? (
          <Spin size="small" style={{ display: 'block', marginTop: 24 }} />
        ) : conversations.length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无对话" />
        ) : (
          Object.entries(groupedConversations()).map(([agentCode, convs]) => (
            <div key={agentCode} style={{ marginBottom: 12 }}>
              <Text type="secondary" style={{ fontSize: 11, paddingLeft: 4 }}>
                {agentCode}
              </Text>
              <List
                size="small"
                dataSource={convs}
                renderItem={(c) => (
                  <List.Item
                    style={{
                      cursor: 'pointer',
                      padding: '6px 8px',
                      background: c.id === conversationId ? '#e6f4ff' : 'transparent',
                      borderRadius: 4,
                    }}
                    onClick={() => navigate(`/conversations/${c.id}`)}
                  >
                    <Space direction="vertical" size={2} style={{ width: '100%' }}>
                      <Text strong style={{ fontSize: 13 }} ellipsis={{ tooltip: c.title }}>
                        {c.title || '未命名对话'}
                      </Text>
                      <Text type="secondary" style={{ fontSize: 11 }}>
                        {c.updated_at ? new Date(c.updated_at).toLocaleString() : ''}
                      </Text>
                    </Space>
                  </List.Item>
                )}
              />
            </div>
          ))
        )}
      </Sider>

      <Content style={{ display: 'flex', flexDirection: 'column', background: '#fafafa' }}>
        <div
          ref={messageListRef}
          style={{
            flex: 1,
            overflow: 'auto',
            padding: 16,
          }}
        >
          <div style={{ maxWidth: 800, margin: '0 auto' }}>
            {messages.length === 0 && !activeRun ? (
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description={
                  <span>
                    <Text type="secondary">开始与智能体对话</Text>
                  </span>
                }
              />
            ) : (
              messages.map((m) => (
                <MessageBubble
                  key={m.id}
                  role={m.role}
                  content={m.content}
                  isStreaming={m.isStreaming}
                  tokens={m.tokens}
                />
              ))
            )}
          </div>
        </div>

        <div
          style={{
            borderTop: '1px solid #f0f0f0',
            background: '#fff',
            padding: '12px 16px',
          }}
        >
          {clarification && (
            <div style={{ maxWidth: 800, margin: '0 auto 12px' }}>
              <ClarificationCard
                schema={clarification.schema}
                onSubmit={handleClarificationSubmit}
                onCancel={handleClarificationCancel}
              />
            </div>
          )}

          {approval && (
            <div style={{ maxWidth: 800, margin: '0 auto 12px' }}>
              <div
                style={{
                  padding: 12,
                  background: '#fffbe6',
                  border: '1px solid #ffe58f',
                  borderRadius: 8,
                }}
              >
                <Space direction="vertical">
                  <Text strong>需要审批</Text>
                  <Text>{approval.title}</Text>
                  <Space>
                    <Tag color="orange">{approval.tool_name}</Tag>
                    <Tag color={approval.status === 'pending' ? 'red' : 'default'}>
                      {approval.status}
                    </Tag>
                  </Space>
                </Space>
              </div>
            </div>
          )}

          <div style={{ maxWidth: 800, margin: '0 auto' }}>
            <Space.Compact style={{ width: '100%' }}>
              <TextArea
                value={inputText}
                onChange={(e) => setInputText(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder={activeRun ? '智能体处理中...' : '输入消息，Enter 发送，Shift+Enter 换行'}
                disabled={activeRun}
                autoSize={{ minRows: 1, maxRows: 6 }}
                style={{ resize: 'none' }}
              />
              {activeRun ? (
                <Button
                  danger
                  icon={<StopOutlined />}
                  onClick={handleCancelRun}
                >
                  取消
                </Button>
              ) : (
                <Button
                  type="primary"
                  icon={<SendOutlined />}
                  onClick={handleSend}
                  disabled={!inputText.trim() || sending}
                  loading={sending}
                >
                  发送
                </Button>
              )}
            </Space.Compact>

            <div style={{ marginTop: 4, display: 'flex', justifyContent: 'space-between' }}>
              <Text type="secondary" style={{ fontSize: 11 }}>
                {connected ? (
                  <Badge color="green" text="已连接" />
                ) : (
                  <Badge color="red" text="连接中断" />
                )}
                {activeRun && <Tag color="processing" style={{ marginLeft: 8 }}>运行中</Tag>}
              </Text>
              <Text type="secondary" style={{ fontSize: 11 }}>
                {streamingMessageId ? '正在生成回复...' : ''}
              </Text>
            </div>
          </div>
        </div>
      </Content>
    </Layout>
  );
}