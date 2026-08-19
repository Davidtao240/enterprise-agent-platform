import { Typography, Tag } from 'antd';

const { Text, Paragraph } = Typography;

interface MessageBubbleProps {
  role: 'user' | 'assistant' | 'system';
  content: string;
  isStreaming?: boolean;
  tokens?: number;
}

export default function MessageBubble({ role, content, isStreaming, tokens }: MessageBubbleProps) {
  const isUser = role === 'user';
  const isSystem = role === 'system';

  if (isSystem) {
    return (
      <div style={{ textAlign: 'center', margin: '8px 0' }}>
        <Tag color="default" style={{ margin: 0 }}>{content}</Tag>
      </div>
    );
  }

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: isUser ? 'row-reverse' : 'row',
        alignItems: 'flex-start',
        marginBottom: 12,
        gap: 8,
      }}
    >
      <div
        style={{
          width: 32,
          height: 32,
          borderRadius: '50%',
          background: isUser ? '#1677ff' : '#f0f0f0',
          color: isUser ? '#fff' : '#666',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          fontSize: 12,
          fontWeight: 600,
          flexShrink: 0,
        }}
      >
        {isUser ? 'U' : 'AI'}
      </div>

      <div
        style={{
          maxWidth: '70%',
          background: isUser ? '#1677ff' : '#f5f5f5',
          color: isUser ? '#fff' : '#333',
          borderRadius: 8,
          padding: '10px 14px',
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-word',
        }}
      >
        <Paragraph
          style={{ margin: 0, color: isUser ? '#fff' : '#333' }}
        >
          {content}
          {isStreaming && (
            <Text
              type="secondary"
              style={{
                marginLeft: 4,
                fontSize: 12,
                color: isUser ? 'rgba(255,255,255,0.7)' : '#999',
              }}
            >
              ▋
            </Text>
          )}
        </Paragraph>
        {!isStreaming && tokens != null && (
          <Text
            style={{
              display: 'block',
              marginTop: 4,
              fontSize: 11,
              color: isUser ? 'rgba(255,255,255,0.6)' : '#999',
            }}
          >
            {tokens} tokens
          </Text>
        )}
      </div>
    </div>
  );
}