import { useCallback, useEffect, useState } from 'react';
import {
  Row, Col, Input, Spin, Empty, Typography, message, Card,
  Button, Tag, Space, Avatar,
} from 'antd';
import { SearchOutlined, ApiOutlined, DownloadOutlined, CheckCircleFilled } from '@ant-design/icons';
import { getConnectorMarket, installConnector, uninstallConnector, MarketItem } from '../../services/marketplace';
import { useAuthStore } from '../../store/auth';

const { Title, Text, Paragraph } = Typography;

const CATEGORY_COLORS: Record<string, string> = {
  erp: 'blue',
  oa: 'green',
  finance: 'gold',
  hr: 'purple',
  procurement: 'orange',
  custom: 'default',
};

export default function ConnectorMarketPage() {
  const token = useAuthStore((s) => s.token);
  const [loading, setLoading] = useState(true);
  const [connectors, setConnectors] = useState<MarketItem[]>([]);
  const [search, setSearch] = useState('');
  const [installing, setInstalling] = useState<string | null>(null);

  const loadConnectors = useCallback(async () => {
    if (!token) return;
    setLoading(true);
    try {
      const result = await getConnectorMarket({ q: search });
      const connList = Array.isArray(result) ? result : (result?.packages || result?.items || result?.data?.packages || result?.data?.items || []);
      setConnectors(connList);
    } catch {
      setConnectors([]);
    } finally {
      setLoading(false);
    }
  }, [token, search]);

  useEffect(() => {
    loadConnectors();
  }, [loadConnectors]);

  const handleInstall = async (item: MarketItem) => {
    setInstalling(item.code);
    try {
      await installConnector(item.code);
      message.success(`已安装连接器 ${item.name}`);
      loadConnectors();
    } catch {
      message.error('安装失败');
    } finally {
      setInstalling(null);
    }
  };

  const handleUninstall = async (item: MarketItem) => {
    try {
      await uninstallConnector(item.code);
      message.success(`已卸载连接器 ${item.name}`);
      loadConnectors();
    } catch {
      message.error('卸载失败');
    }
  };

  return (
    <div>
      <Title level={4}>连接器市场</Title>
      <Paragraph type="secondary">
        连接器用于对接外部系统（ERP、OA、财务系统等）。安装后智能体可通过 Connector Protocol 调用外部能力。
      </Paragraph>

      <Card size="small" style={{ marginBottom: 16 }} styles={{ body: { padding: 12 } }}>
        <Input
          prefix={<SearchOutlined />}
          placeholder="搜索连接器..."
          allowClear
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          onPressEnter={() => loadConnectors()}
        />
      </Card>

      <Spin spinning={loading}>
        {connectors.length === 0 ? (
          <Empty description="暂无可用连接器" />
        ) : (
          <Row gutter={[16, 16]}>
            {connectors.map((c) => (
              <Col xs={24} sm={12} lg={8} key={c.id}>
                <Card
                  hoverable
                  extra={
                    <Space>
                      <Tag color={CATEGORY_COLORS[c.category] || 'default'}>
                        {c.category}
                      </Tag>
                      {c.installed && (
                        <Tag color="success" icon={<CheckCircleFilled />}>
                          已安装
                        </Tag>
                      )}
                    </Space>
                  }
                  actions={[
                    c.installed ? (
                      <Button key="uninstall" danger size="small" onClick={() => handleUninstall(c)}>
                        卸载
                      </Button>
                    ) : (
                      <Button
                        key="install"
                        type="primary"
                        size="small"
                        icon={<DownloadOutlined />}
                        loading={installing === c.code}
                        onClick={() => handleInstall(c)}
                      >
                        安装
                      </Button>
                    ),
                  ]}
                >
                  <Card.Meta
                    avatar={
                      <Avatar
                        shape="square"
                        style={{ backgroundColor: '#52c41a20' }}
                        icon={<ApiOutlined />}
                      />
                    }
                    title={<Text strong>{c.name}</Text>}
                    description={
                      <div>
                        <Paragraph ellipsis={{ rows: 2 }} type="secondary" style={{ marginBottom: 8 }}>
                          {c.description}
                        </Paragraph>
                        <Space size="small" style={{ fontSize: 12 }}>
                          <Text type="secondary">v{c.version}</Text>
                          <Text type="secondary">·</Text>
                          <Text type="secondary">{c.usage_count?.toLocaleString() || 0} 已安装</Text>
                          {c.author && (
                            <>
                              <Text type="secondary">·</Text>
                              <Text type="secondary">{c.author}</Text>
                            </>
                          )}
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
    </div>
  );
}