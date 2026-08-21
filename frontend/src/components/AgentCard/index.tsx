import { Card, Tag, Typography, Space, Button, Tooltip } from 'antd';
import { MessageOutlined, ThunderboltFilled, ClockCircleOutlined, ArrowRightOutlined } from '@ant-design/icons';
import type { AgentPackageListItem } from '../../services/gallery';
import AgentIcon, { getCategoryLabel, getCategoryGradient } from './AgentIcon';

const { Text } = Typography;

interface AgentCardProps {
  pkg: AgentPackageListItem;
  onClick?: (pkg: AgentPackageListItem) => void;
}

function formatRelativeTime(dateStr?: string): string {
  if (!dateStr) return '暂无使用记录';
  const diff = Date.now() - new Date(dateStr).getTime();
  const minutes = Math.floor(diff / 60000);
  const hours = Math.floor(diff / 3600000);
  const days = Math.floor(diff / 86400000);

  if (minutes < 1) return '刚刚使用';
  if (minutes < 60) return `${minutes} 分钟前`;
  if (hours < 24) return `${hours} 小时前`;
  if (days < 30) return `${days} 天前`;
  return new Date(dateStr).toLocaleDateString();
}

function formatUsageCount(count: number): string {
  if (count >= 10000) return `${(count / 10000).toFixed(1)}w`;
  if (count >= 1000) return `${(count / 1000).toFixed(1)}k`;
  return String(count);
}

export default function AgentCard({ pkg, onClick }: AgentCardProps) {
  const handleStartChat = (e: React.MouseEvent) => {
    e.stopPropagation();
    onClick?.(pkg);
  };

  const gradient = getCategoryGradient(pkg.category);
  const categoryLabel = getCategoryLabel(pkg.category);
  const usageCount = pkg.usage_count ?? 0;

  return (
    <Card
      hoverable
      onClick={() => onClick?.(pkg)}
      className="agent-card-v2"
      style={{
        height: '100%',
        cursor: 'pointer',
        overflow: 'hidden',
        position: 'relative',
      }}
      styles={{ body: { padding: 0 } }}
    >
      {/* 顶部彩色渐变状态条 */}
      <div
        className="agent-card-top-bar"
        style={{ background: gradient }}
      />

      <div style={{ padding: 16 }}>
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          {/* 头部：图标 + 名称 + 热度 */}
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', width: '100%' }}>
            <Space align="start" size={12}>
              <AgentIcon category={pkg.category} size={48} iconSize={22} />
              <div style={{ minWidth: 0 }}>
                <Tooltip title={pkg.name}>
                  <Text
                    strong
                    style={{
                      fontSize: 15,
                      lineHeight: 1.4,
                      display: 'block',
                      whiteSpace: 'nowrap',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      maxWidth: 160,
                    }}
                  >
                    {pkg.name}
                  </Text>
                </Tooltip>
                <Tag
                  className="agent-card-category-tag"
                  style={{
                    marginTop: 6,
                    color: gradient.split(',')[0].replace('linear-gradient(135deg, ', '').trim(),
                  }}
                >
                  {categoryLabel}
                </Tag>
              </div>
            </Space>
            {usageCount > 0 && (
              <Tooltip title={`${usageCount} 次使用`}>
                <div
                  className="agent-card-usage"
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 4,
                    padding: '2px 8px',
                    borderRadius: 12,
                    background: 'rgba(0,0,0,0.04)',
                    fontSize: 12,
                    color: 'var(--text-secondary)',
                  }}
                >
                  <ThunderboltFilled style={{ color: '#F59E0B', fontSize: 12 }} />
                  <Text style={{ fontSize: 12, color: 'inherit' }}>{formatUsageCount(usageCount)}</Text>
                </div>
              </Tooltip>
            )}
          </div>

          {/* 描述 */}
          <Text
            type="secondary"
            style={{
              fontSize: 13,
              lineHeight: 1.6,
              display: '-webkit-box',
              WebkitLineClamp: 2,
              WebkitBoxOrient: 'vertical',
              overflow: 'hidden',
              minHeight: 42,
            }}
          >
            {pkg.description || '暂无描述'}
          </Text>

          {/* 底部：使用时间 + 开始对话 */}
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              width: '100%',
              paddingTop: 12,
              borderTop: '1px solid var(--neutral-100)',
            }}
          >
            <Text type="secondary" style={{ fontSize: 12, display: 'flex', alignItems: 'center', gap: 4 }}>
              <ClockCircleOutlined style={{ fontSize: 12 }} />
              {formatRelativeTime(pkg.last_used_at)}
            </Text>
            <Button
              type="primary"
              size="small"
              icon={<MessageOutlined />}
              onClick={handleStartChat}
              className="agent-card-chat-btn"
            >
              开始对话
              <ArrowRightOutlined style={{ fontSize: 11 }} />
            </Button>
          </div>
        </Space>
      </div>
    </Card>
  );
}
