import { Card, Tag, Typography, Space } from 'antd';
import { RobotOutlined } from '@ant-design/icons';
import type { AgentPackageListItem } from '../../services/gallery';

const { Text } = Typography;

interface AgentCardProps {
  pkg: AgentPackageListItem;
  onClick?: (pkg: AgentPackageListItem) => void;
}

const categoryColorMap: Record<string, string> = {
  general: 'blue',
  departmental: 'purple',
  finance: 'green',
};

export default function AgentCard({ pkg, onClick }: AgentCardProps) {
  const handleClick = () => {
    onClick?.(pkg);
  };

  return (
    <Card
      hoverable
      onClick={handleClick}
      style={{ height: '100%', cursor: 'pointer' }}
      styles={{ body: { padding: 16 } }}
    >
      <Space direction="vertical" size={8} style={{ width: '100%' }}>
        <Space align="start" style={{ width: '100%', justifyContent: 'space-between' }}>
          <Space align="start">
            <div
              style={{
                width: 40,
                height: 40,
                borderRadius: 8,
                background: '#e6f4ff',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                fontSize: 20,
              }}
            >
              {pkg.icon || <RobotOutlined style={{ color: '#1677ff' }} />}
            </div>
            <div>
              <Text strong style={{ fontSize: 15 }} ellipsis={{ tooltip: pkg.name }}>
                {pkg.name}
              </Text>
              <div>
                <Tag
                  color={categoryColorMap[pkg.category] || 'default'}
                  style={{ marginTop: 4 }}
                >
                  {pkg.category === 'general' ? '通用' : pkg.category === 'departmental' ? '部门级' : pkg.category}
                </Tag>
              </div>
            </div>
          </Space>
        </Space>

        <Text
          type="secondary"
          style={{
            fontSize: 13,
            display: '-webkit-box',
            WebkitLineClamp: 2,
            WebkitBoxOrient: 'vertical',
            overflow: 'hidden',
          }}
        >
          {pkg.description}
        </Text>

        <Text type="secondary" style={{ fontSize: 12 }}>
          使用次数: {pkg.usage_count ?? 0}
        </Text>
      </Space>
    </Card>
  );
}