import { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Row, Col, Tabs, Input, Spin, Empty, Typography, message } from 'antd';
import { SearchOutlined } from '@ant-design/icons';
import AgentCard from '../../components/AgentCard';
import { getAgentGallery, AgentPackageListItem } from '../../services/gallery';
import { createConversation } from '../../services/conversation';
import { useAuthStore } from '../../store/auth';

const { Title } = Typography;

type CategoryTab = 'all' | 'general' | 'departmental';

const categoryTabItems: { key: CategoryTab; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'general', label: '通用' },
  { key: 'departmental', label: '部门级' },
];

export default function AgentGalleryPage() {
  const navigate = useNavigate();
  const token = useAuthStore((s) => s.token);

  const [loading, setLoading] = useState(true);
  const [packages, setPackages] = useState<AgentPackageListItem[]>([]);
  const [category, setCategory] = useState<CategoryTab>('all');
  const [search, setSearch] = useState('');

  const loadGallery = useCallback(async () => {
    if (!token) return;
    setLoading(true);
    try {
      const params: { category?: string; q?: string } = {};
      if (category !== 'all') params.category = category;
      if (search.trim()) params.q = search.trim();
      const { data } = await getAgentGallery(params);
      setPackages(data.data?.packages || []);
    } catch {
      setPackages([]);
    } finally {
      setLoading(false);
    }
  }, [token, category, search]);

  useEffect(() => {
    loadGallery();
  }, [loadGallery]);

  const handleCardClick = async (pkg: AgentPackageListItem) => {
    try {
      const { data } = await createConversation(pkg.package_code);
      navigate(`/conversations/${data.data.id}`);
    } catch {
      message.error('创建对话失败');
    }
  };

  return (
    <div>
      <Title level={4}>智能体广场</Title>

      <Tabs
        activeKey={category}
        onChange={(key) => setCategory(key as CategoryTab)}
        items={categoryTabItems}
      />

      <Input
        allowClear
        placeholder="搜索智能体..."
        prefix={<SearchOutlined />}
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        style={{ maxWidth: 400, marginBottom: 16 }}
      />

      {loading ? (
        <Spin size="large" style={{ display: 'block', margin: '80px auto' }} />
      ) : packages.length === 0 ? (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description="暂无智能体"
          style={{ marginTop: 60 }}
        />
      ) : (
        <Row gutter={[16, 16]}>
          {packages.map((pkg) => (
            <Col key={pkg.package_code} xs={24} sm={12} md={8} lg={6}>
              <AgentCard pkg={pkg} onClick={handleCardClick} />
            </Col>
          ))}
        </Row>
      )}
    </div>
  );
}