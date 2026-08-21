import { Typography, Tag } from 'antd';
import MarkdownContent from '../MarkdownContent';

const { Text } = Typography;

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
          background: isUser ? 'var(--color-primary-500)' : 'linear-gradient(135deg, #667EEA 0%, #764BA2 100%)',
          color: '#fff',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          fontSize: 12,
          fontWeight: 600,
          flexShrink: 0,
          boxShadow: isUser ? 'none' : '0 2px 8px rgba(102, 126, 234, 0.35)',
        }}
      >
        {isUser ? '我' : 'AI'}
      </div>

      <div
        className="message-bubble"
        style={{
          maxWidth: '80%',
          background: isUser ? 'var(--color-primary-500)' : 'var(--neutral-100)',
          color: isUser ? '#fff' : 'var(--text-primary)',
          borderRadius: 'var(--radius-lg)',
          borderBottomRightRadius: isUser ? 'var(--radius-sm)' : undefined,
          borderBottomLeftRadius: isUser ? undefined : 'var(--radius-sm)',
          padding: '10px 14px',
          wordBreak: 'break-word',
        }}
      >
        {isUser ? (
          <Text style={{ margin: 0, color: '#fff', whiteSpace: 'pre-wrap' }}>
            {content}
          </Text>
        ) : (
          <>
            <MarkdownContent content={content} />
            {isStreaming && (
              <Text
                type="secondary"
                style={{
                  marginLeft: 4,
                  fontSize: 12,
                  color: '#666',
                }}
              >
                ▋
              </Text>
            )}
          </>
        )}
        {!isStreaming && tokens != null && (
          <Text
            style={{
              display: 'block',
              marginTop: 4,
              fontSize: 11,
              color: isUser ? 'rgba(255,255,255,0.6)' : 'var(--text-tertiary)',
            }}
          >
            {tokens} tokens
          </Text>
        )}
      </div>
    </div>
  );
}
