import { useCallback, useEffect, useState } from 'react';
import {
  Row, Col, Tabs, Input, Spin, Empty, Typography, message, Card,
  Button, Tag, Rate, Space, Modal, List, Avatar, Divider,
} from 'antd';
import {
  SearchOutlined, DownloadOutlined,
  StarFilled, UserOutlined,
  AppstoreOutlined, ApiOutlined, BulbOutlined,
  CheckCircleFilled,
} from '@ant-design/icons';
import {
  getMarketItems, installMarketItem, uninstallMarketItem,
  rateMarketItem, getMarketReviews, MarketItem, MarketReview,
} from '../../services/marketplace';
import { useAuthStore } from '../../store/auth';

const { Text, Paragraph } = Typography;
const { TextArea } = Input;

type MarketTab = 'agent' | 'connector' | 'skill';
type SortOption = 'rating' | 'usage' | 'newest';

const TAB_ITEMS: { key: MarketTab; label: string; icon: React.ReactNode; color: string }[] = [
  { key: 'agent', label: '智能体', icon: <AppstoreOutlined />, color: 'blue' },
  { key: 'connector', label: '连接器', icon: <ApiOutlined />, color: 'green' },
  { key: 'skill', label: '技能', icon: <BulbOutlined />, color: 'orange' },
];

const TAB_GRADIENTS: Record<MarketTab, string> = {
  agent: 'linear-gradient(135deg, #667EEA 0%, #764BA2 100%)',
  connector: 'linear-gradient(135deg, #4FACFE 0%, #00F2FE 100%)',
  skill: 'linear-gradient(135deg, #F6D365 0%, #FDA085 100%)',
};

export default function MarketplacePage() {
  const token = useAuthStore((s) => s.token);
  const [loading, setLoading] = useState(true);
  const [items, setItems] = useState<MarketItem[]>([]);
  const [tab, setTab] = useState<MarketTab>('agent');
  const [search, setSearch] = useState('');
  const [sort, setSort] = useState<SortOption>('rating');
  const [installing, setInstalling] = useState<string | null>(null);
  const [uninstalling, setUninstalling] = useState<string | null>(null);

  const [reviewModalOpen, setReviewModalOpen] = useState(false);
  const [reviewTarget, setReviewTarget] = useState<MarketItem | null>(null);
  const [reviewRating, setReviewRating] = useState(5);
  const [reviewComment, setReviewComment] = useState('');
  const [reviews, setReviews] = useState<MarketReview[]>([]);
  const [reviewsLoading, setReviewsLoading] = useState(false);

  const loadItems = useCallback(async () => {
    if (!token) return;
    setLoading(true);
    try {
      const result = await getMarketItems({ type: tab, q: search, sort });
      const itemList = Array.isArray(result) ? result : (result?.packages || result?.items || result?.data?.packages || result?.data?.items || []);
      setItems(itemList);
    } catch {
      setItems([]);
    } finally {
      setLoading(false);
    }
  }, [token, tab, search, sort]);

  useEffect(() => {
    loadItems();
  }, [loadItems]);

  const handleInstall = async (item: MarketItem) => {
    setInstalling(item.code);
    try {
      await installMarketItem(item.code);
      message.success(`已安装 ${item.name}`);
      loadItems();
    } catch {
      message.error('安装失败');
    } finally {
      setInstalling(null);
    }
  };

  const handleUninstall = async (item: MarketItem) => {
    setUninstalling(item.code);
    try {
      await uninstallMarketItem(item.code);
      message.success(`已卸载 ${item.name}`);
      loadItems();
    } catch {
      message.error('卸载失败');
    } finally {
      setUninstalling(null);
    }
  };

  const handleOpenReview = async (item: MarketItem) => {
    setReviewTarget(item);
    setReviewRating(5);
    setReviewComment('');
    setReviewModalOpen(true);
    setReviewsLoading(true);
    try {
      const result = await getMarketReviews(item.code);
      const reviewList = Array.isArray(result) ? result : (result?.reviews || result?.data?.reviews || []);
      setReviews(reviewList);
    } catch {
      setReviews([]);
    } finally {
      setReviewsLoading(false);
    }
  };

  const handleSubmitReview = async () => {
    if (!reviewTarget) return;
    try {
      await rateMarketItem(reviewTarget.code, { rating: reviewRating, comment: reviewComment });
      message.success('评价提交成功');
      setReviewModalOpen(false);
      loadItems();
    } catch {
      message.error('评价提交失败');
    }
  };

  return (
    <div className="fade-in">
      <div className="page-header">
        <h1 className="page-title">市场</h1>
        <p className="page-subtitle">
          浏览和安装智能体、连接器和技能。安装后可在工作台中使用。
        </p>
      </div>

      <Card
        size="small"
        style={{ marginBottom: 16 }}
        styles={{ body: { padding: 12 } }}
      >
        <Row gutter={16} align="middle">
          <Col flex="auto">
            <Input
              prefix={<SearchOutlined />}
              placeholder={`搜索${tab === 'agent' ? '智能体' : tab === 'connector' ? '连接器' : '技能'}...`}
              allowClear
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              onPressEnter={() => loadItems()}
            />
          </Col>
          <Col>
            <Space>
              <Text type="secondary">排序:</Text>
              <Button.Group>
                {(['rating', 'usage', 'newest'] as SortOption[]).map((s) => (
                  <Button
                    key={s}
                    type={sort === s ? 'primary' : 'default'}
                    size="small"
                    onClick={() => setSort(s)}
                  >
                    {s === 'rating' ? '评分' : s === 'usage' ? '用量' : '最新'}
                  </Button>
                ))}
              </Button.Group>
            </Space>
          </Col>
        </Row>
      </Card>

      <Tabs
        activeKey={tab}
        onChange={(k) => setTab(k as MarketTab)}
        items={TAB_ITEMS.map((t) => ({
          key: t.key,
          label: (
            <span>
              {t.icon} {t.label}
            </span>
          ),
        }))}
      />

      <Spin spinning={loading}>
        {items.length === 0 ? (
          <Empty description="暂无可用项目" />
        ) : (
          <Row gutter={[16, 16]}>
            {items.map((item) => (
              <Col xs={24} sm={12} lg={8} xl={6} key={item.id}>
                <Card
                  hoverable
                  actions={[
                    item.installed ? (
                      <Button
                        key="uninstall"
                        danger
                        size="small"
                        loading={uninstalling === item.code}
                        onClick={() => handleUninstall(item)}
                      >
                        卸载
                      </Button>
                    ) : (
                      <Button
                        key="install"
                        type="primary"
                        size="small"
                        icon={<DownloadOutlined />}
                        loading={installing === item.code}
                        onClick={() => handleInstall(item)}
                      >
                        安装
                      </Button>
                    ),
                    <Button
                      key="review"
                      size="small"
                      icon={<StarFilled />}
                      onClick={() => handleOpenReview(item)}
                    >
                      评价
                    </Button>,
                  ]}
                >
                  <Card.Meta
                    avatar={
                      <div
                        className="dashboard-stat-icon"
                        style={{ background: TAB_GRADIENTS[tab] }}
                      >
                        {TAB_ITEMS.find((t) => t.key === tab)?.icon}
                      </div>
                    }
                    title={
                      <Space>
                        {item.name}
                        {item.installed && (
                          <Tag color="success" icon={<CheckCircleFilled />}>
                            已安装
                          </Tag>
                        )}
                      </Space>
                    }
                    description={
                      <div>
                        <Paragraph
                          ellipsis={{ rows: 2 }}
                          type="secondary"
                          style={{ marginBottom: 8 }}
                        >
                          {item.description}
                        </Paragraph>
                        <Space size="small" style={{ fontSize: 12 }}>
                          <Rate
                            disabled
                            allowHalf
                            value={item.rating}
                            style={{ fontSize: 12 }}
                          />
                          <Text type="secondary">{item.rating?.toFixed(1) || '暂无'}</Text>
                          <Text type="secondary">·</Text>
                          <Text type="secondary">{item.usage_count?.toLocaleString() || 0} 使用</Text>
                          <Text type="secondary">·</Text>
                          <Tag>{item.version}</Tag>
                        </Space>
                      </div>
                    }
                  />
                </Card>
              </Col>
            ))}
          </Row>
        )}
      </Spin>

      <Modal
        title={reviewTarget ? `${reviewTarget.name} - 用户评价` : '评价'}
        open={reviewModalOpen}
        onCancel={() => setReviewModalOpen(false)}
        onOk={handleSubmitReview}
        okText="提交评价"
        cancelText="取消"
      >
        {reviewTarget && (
          <div>
            <div style={{ marginBottom: 16 }}>
              <Text strong>您的评分:</Text>
              <br />
              <Rate value={reviewRating} onChange={setReviewRating} style={{ fontSize: 24 }} />
            </div>
            <div style={{ marginBottom: 16 }}>
              <Text strong>您的评价:</Text>
              <TextArea
                rows={3}
                placeholder="分享您的使用体验..."
                value={reviewComment}
                onChange={(e) => setReviewComment(e.target.value)}
              />
            </div>
            <Divider>所有评价</Divider>
            <Spin spinning={reviewsLoading}>
              {reviews.length === 0 ? (
                <Empty description="暂无评价，成为第一个评价的人吧" />
              ) : (
                <List
                  size="small"
                  dataSource={reviews}
                  renderItem={(r) => (
                    <List.Item>
                      <List.Item.Meta
                        avatar={<Avatar icon={<UserOutlined />} />}
                        title={
                          <Space>
                            <Text strong>{r.user_name}</Text>
                            <Rate disabled allowHalf value={r.rating} style={{ fontSize: 12 }} />
                          </Space>
                        }
                        description={
                          <div>
                            {r.comment && <div>{r.comment}</div>}
                            <Text type="secondary" style={{ fontSize: 12 }}>
                              {new Date(r.created_at).toLocaleString('zh-CN')}
                            </Text>
                          </div>
                        }
                      />
                    </List.Item>
                  )}
                />
              )}
            </Spin>
          </div>
        )}
      </Modal>
    </div>
  );
}