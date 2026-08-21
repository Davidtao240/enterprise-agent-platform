import { RobotOutlined, TeamOutlined, MoneyCollectOutlined, FileTextOutlined, AuditOutlined, LineChartOutlined, ApiOutlined, BulbOutlined, CalculatorOutlined } from '@ant-design/icons';
import { categoryGradients, categoryLabels } from '../../styles/theme';

// 每个分类的专属图标
const categoryIcons: Record<string, React.ReactNode> = {
  general: <RobotOutlined />,
  departmental: <TeamOutlined />,
  finance: <MoneyCollectOutlined />,
  document: <FileTextOutlined />,
  audit: <AuditOutlined />,
  planning: <LineChartOutlined />,
  connector: <ApiOutlined />,
  skill: <BulbOutlined />,
  tax: <CalculatorOutlined />,
};

// 分类 → 渐变背景 (从 theme.ts 导入，含 fallback)
const categoryGradientMap: Record<string, string> = {
  general: categoryGradients.general,
  departmental: categoryGradients.general,
  finance: categoryGradients.finance,
  document: categoryGradients.document,
  audit: categoryGradients.audit,
  planning: categoryGradients.planning,
  connector: categoryGradients.connector,
  skill: categoryGradients.skill,
  tax: categoryGradients.finance,
};

export function getCategoryLabel(category: string): string {
  return categoryLabels[category] || category || '通用';
}

export function getCategoryGradient(category: string): string {
  return categoryGradientMap[category] || categoryGradients.general;
}

export function getCategoryIcon(category: string, size = 24): React.ReactNode {
  const icon = categoryIcons[category] || <RobotOutlined />;
  return <span style={{ fontSize: size, lineHeight: 1 }}>{icon}</span>;
}

export default function AgentIcon({
  category,
  size = 48,
  iconSize = 24,
}: {
  category: string;
  size?: number;
  iconSize?: number;
}) {
  return (
    <div
      className="agent-card-icon"
      style={{
        width: size,
        height: size,
        borderRadius: size * 0.25,
        background: getCategoryGradient(category),
        boxShadow: `0 4px 12px ${getCategoryGradient(category)}55`,
      }}
    >
      {getCategoryIcon(category, iconSize)}
    </div>
  );
}
