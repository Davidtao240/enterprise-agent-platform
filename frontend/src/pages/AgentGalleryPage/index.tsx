import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Row, Col, Input, Spin, Empty, Typography, message, Button, Segmented, Tooltip } from 'antd';
import { SearchOutlined, ReloadOutlined, ThunderboltOutlined } from '@ant-design/icons';
import AgentCard from '../../components/AgentCard';
import { getAgentGallery, AgentPackageListItem } from '../../services/gallery';
import { createConversation } from '../../services/conversation';
import { useAuthStore } from '../../store/auth';
import { safeRequest } from '../../utils/errorHandler';
import { getCategoryLabel } from '../../components/AgentCard/AgentIcon';

const { Title, Text } = Typography;

const SAMPLE_PACKAGES: AgentPackageListItem[] = [
  {
    id: '1',
    package_code: 'financial_analysis',
    name: '财务分析助手',
    description: '智能分析财务数据，生成专业分析报告',
    category: 'finance',
    business_app_code: 'finance',
    status: 'active',
    usage_count: 156,
    last_used_at: new Date().toISOString(),
  },
  {
    id: '2',
    package_code: 'invoice_processor',
    name: '发票处理专家',
    description: '自动识别和处理各类发票，提取关键信息',
    category: 'document',
    business_app_code: 'finance',
    status: 'active',
    usage_count: 89,
    last_used_at: new Date(Date.now() - 86400000).toISOString(),
  },
  {
    id: '3',
    package_code: 'expense_reviewer',
    name: '费用审核员',
    description: '自动审核费用报销，合规性检查和异常检测',
    category: 'audit',
    business_app_code: 'finance',
    status: 'active',
    usage_count: 234,
    last_used_at: new Date(Date.now() - 172800000).toISOString(),
  },
  {
    id: '4',
    package_code: 'budget_planner',
    name: '预算规划师',
    description: '智能预算编制、执行监控和偏差分析',
    category: 'planning',
    business_app_code: 'finance',
    status: 'active',
    usage_count: 67,
    last_used_at: new Date(Date.now() - 259200000).toISOString(),
  },
];

export default function AgentGalleryPage() {
  const navigate = useNavigate();
  const token = useAuthStore((s) => s.token);

  const [loading, setLoading] = useState(true);
  const [allPackages, setAllPackages] = useState<AgentPackageListItem[]>([]);
  const [category, setCategory] = useState<string>('all');
  const [search, setSearch] = useState('');
  const [loadError, setLoadError] = useState(false);

  const loadGallery = useCallback(async () => {
    if (!token) return;
    setLoading(true);
    setLoadError(false);
    try {
      const result = await safeRequest(
        () => getAgentGallery(),
        SAMPLE_PACKAGES,
        { showErrorMessage: false }
      );
      const pkgList = Array.isArray(result) ? result : (result?.packages || result?.data?.packages || SAMPLE_PACKAGES);
      setAllPackages(pkgList.length > 0 ? pkgList : SAMPLE_PACKAGES);
    } catch {
      setLoadError(true);
      setAllPackages(SAMPLE_PACKAGES);
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => {
    loadGallery();
  }, [loadGallery]);

  // 动态提取分类列表（从数据中出现过的分类生成）
  const categories = useMemo(() => {
    const set = new Set<string>();
    allPackages.forEach((p) => set.add(p.category));
    return Array.from(set);
  }, [allPackages]);

  // 客户端过滤：分类 + 搜索
  const filteredPackages = useMemo(() => {
    return allPackages.filter((pkg) => {
      const matchCategory = category === 'all' || pkg.category === category;
      const q = search.trim().toLowerCase();
      const matchSearch = !q
        || pkg.name.toLowerCase().includes(q)
        || (pkg.description || '').toLowerCase().includes(q)
        || (pkg.package_code || '').toLowerCase().includes(q);
      return matchCategory && matchSearch;
    });
  }, [allPackages, category, search]);

  const handleCardClick = async (pkg: AgentPackageListItem) => {
    try {
      const conv = await createConversation(pkg.package_code);
      navigate(`/conversations/${conv.id}`);
    } catch {
      message.error('创建对话失败，请重试');
    }
  };

  const totalUsage = useMemo(
    () => allPackages.reduce((sum, p) => sum + (p.usage_count || 0), 0),
    [allPackages]
  );

  const segmentOptions = useMemo(() => [
    { label: '全部', value: 'all' },
    ...categories.map((c) => ({ label: getCategoryLabel(c), value: c })),
  ], [categories]);

  return (
    <div>
      {/* 页面头部 */}
      <div style={{ marginBottom: 20, display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <div>
          <Title level={3} style={{ margin: 0 }}>智能体广场</Title>
          <Text type="secondary">
            选择智能体开始对话，探索企业智能化能力
          </Text>
        </div>
        <div style={{ display: 'flex', gap: 16 }}>
          <div style={{ textAlign: 'right' }}>
            <Text type="secondary" style={{ fontSize: 12, display: 'block' }}>智能体总数</Text>
            <Text strong style={{ fontSize: 20, color: 'var(--color-primary-500)' }}>{allPackages.length}</Text>
          </div>
          <div style={{ textAlign: 'right' }}>
            <Text type="secondary" style={{ fontSize: 12, display: 'block' }}>累计调用</Text>
            <Text strong style={{ fontSize: 20, color: 'var(--color-warning)' }}>{totalUsage.toLocaleString()}</Text>
          </div>
        </div>
      </div>

      {loadError && (
        <div
          style={{
            marginBottom: 16,
            padding: '8px 16px',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            background: 'var(--color-warning)14',
            border: '1px solid #FDE68A',
            borderRadius: 8,
          }}
        >
          <Text style={{ color: '#92400E', fontSize: 13 }}>数据加载失败，正在使用示例数据展示。</Text>
          <Button size="small" icon={<ReloadOutlined />} onClick={() => loadGallery()}>
            重新加载
          </Button>
        </div>
      )}

      {/* 分类 + 搜索 */}
      <div
        style={{
          marginBottom: 20,
          padding: 12,
          background: 'var(--bg-container)',
          borderRadius: 12,
          border: '1px solid var(--neutral-100)',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          flexWrap: 'wrap',
          gap: 12,
        }}
      >
        <Segmented
          options={segmentOptions}
          value={category}
          onChange={(v) => setCategory(v as string)}
        />
        <Input
          allowClear
          placeholder="搜索智能体..."
          prefix={<SearchOutlined style={{ color: 'var(--neutral-400)' }} />}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          style={{ width: 280 }}
        />
      </div>

      {/* 结果区 */}
      {loading ? (
        <Spin size="large" style={{ display: 'block', margin: '80px auto' }} />
      ) : filteredPackages.length === 0 ? (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description="未找到匹配的智能体"
          style={{ marginTop: 60 }}
        >
          <Button onClick={() => { setSearch(''); setCategory('all'); }}>清除筛选</Button>
        </Empty>
      ) : (
        <>
          <Row gutter={[16, 16]}>
            {filteredPackages.map((pkg) => (
              <Col key={pkg.package_code} xs={24} sm={12} md={8} lg={6}>
                <AgentCard pkg={pkg} onClick={handleCardClick} />
              </Col>
            ))}
          </Row>
          <div style={{ textAlign: 'center', marginTop: 24 }}>
            <Tooltip title="使用新技术构建的智能体">
              <Text type="secondary" style={{ fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                <ThunderboltOutlined />
                共 {filteredPackages.length} 个智能体
              </Text>
            </Tooltip>
          </div>
        </>
      )}
    </div>
  );
}
