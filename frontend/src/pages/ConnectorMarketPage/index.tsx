import { useCallback, useEffect, useState } from 'react';
import {
  Row, Col, Input, Spin, Empty, Typography, message, Card,
  Button, Tag, Space,
} from 'antd';
import { SearchOutlined, ApiOutlined, DownloadOutlined, CheckCircleFilled } from '@ant-design/icons';
import { getConnectorMarket, installConnector, uninstallConnector, MarketItem } from '../../services/marketplace';
import { useAuthStore } from '../../store/auth';

const { Text, Paragraph } = Typography;

const CATEGORY_COLORS: Record<string, string> = {
  erp: 'blue',
  oa: 'green',
  finance: 'gold',
  hr: 'purple',
  procurement: 'orange',
  custom: 'default',
};

const CATEGORY_GRADIENTS: Record<string, string> = {
  erp: 'linear-gradient(135deg, #4FACFE 0%, #00F2FE 100%)',
  oa: 'linear-gradient(135deg, #11998E 0%, #38EF7D 100%)',
  finance: 'linear-gradient(135deg, #F6D365 0%, #FDA085 100%)',
  hr: 'linear-gradient(135deg, #667EEA 0%, #764BA2 100%)',
  procurement: 'linear-gradient(135deg, #F093FB 0%, #F5576C 100%)',
  custom: 'linear-gradient(135deg, #FC466B 0%, #3F5EFB 100%)',
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
    <div className="fade-in">
      <div className="page-header">
        <h1 className="page-title">连接器市场</h1>
        <p className="page-subtitle">
          连接器用于对接外部系统（ERP、OA、财务系统等）。安装后智能体可通过 Connector Protocol 调用外部能力。
        </p>
      </div>

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
                  className="connector-card"
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
                      <div
                        className="dashboard-stat-icon"
                        style={{ background: CATEGORY_GRADIENTS[c.category] || CATEGORY_GRADIENTS.custom }}
                      >
                        <ApiOutlined />
                      </div>
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