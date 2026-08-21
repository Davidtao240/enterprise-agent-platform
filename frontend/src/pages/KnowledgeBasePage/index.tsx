import { useCallback, useEffect, useState } from 'react';
import {
  Button,
  Card,
  Col,
  Empty,
  Input,
  Modal,
  Row,
  Space,
  Table,
  Tag,
  Typography,
  Upload,
  message,
} from 'antd';
import {
  PlusOutlined,
  SearchOutlined,
  UploadOutlined,
  DatabaseOutlined,
  DeleteOutlined,
  InboxOutlined,
} from '@ant-design/icons';
import {
  KBCollection,
  KBDocument,
  KBSearchResult,
  knowledgeService,
} from '../../services/knowledge';

const { Title, Text } = Typography;

type ViewMode = 'collections' | 'documents' | 'search';

const COLLECTION_GRADIENTS = [
  'linear-gradient(135deg, #4FACFE 0%, #00F2FE 100%)',
  'linear-gradient(135deg, #F093FB 0%, #F5576C 100%)',
  'linear-gradient(135deg, #11998E 0%, #38EF7D 100%)',
  'linear-gradient(135deg, #F6D365 0%, #FDA085 100%)',
  'linear-gradient(135deg, #667EEA 0%, #764BA2 100%)',
  'linear-gradient(135deg, #FC466B 0%, #3F5EFB 100%)',
];

export default function KnowledgeBasePage() {
  const [collections, setCollections] = useState<KBCollection[]>([]);
  const [currentCollection, setCurrentCollection] = useState<KBCollection | null>(null);
  const [documents, setDocuments] = useState<KBDocument[]>([]);
  const [view, setView] = useState<ViewMode>('collections');
  const [searchResults, setSearchResults] = useState<KBSearchResult[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [loading, setLoading] = useState(false);
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [uploadModalOpen, setUploadModalOpen] = useState(false);
  const [createName, setCreateName] = useState('');
  const [createDesc, setCreateDesc] = useState('');
  const [fileList, setFileList] = useState<any[]>([]);

  const loadCollections = useCallback(async () => {
    setLoading(true);
    try {
      const items = await knowledgeService.listCollections();
      setCollections(items);
    } catch {
      setCollections([]);
    } finally {
      setLoading(false);
    }
  }, []);

  const loadDocuments = useCallback(async (collId: string) => {
    setLoading(true);
    try {
      const items = await knowledgeService.listDocuments(collId);
      setDocuments(items);
    } catch {
      setDocuments([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadCollections();
  }, [loadCollections]);

  useEffect(() => {
    if (currentCollection) {
      loadDocuments(currentCollection.id);
    }
  }, [currentCollection, loadDocuments]);

  const handleCreateCollection = async () => {
    if (!createName.trim()) {
      message.error('请输入知识库名称');
      return;
    }
    try {
      await knowledgeService.createCollection({ name: createName.trim(), description: createDesc.trim() });
      message.success('知识库创建成功');
      setCreateModalOpen(false);
      setCreateName('');
      setCreateDesc('');
      loadCollections();
    } catch {
      message.error('创建失败');
    }
  };

  const handleDeleteCollection = async (id: string) => {
    Modal.confirm({
      title: '确认删除知识库',
      content: '删除后无法恢复，该知识库下所有文档也将被删除。',
      okType: 'danger',
      onOk: async () => {
        try {
          await knowledgeService.deleteCollection(id);
          message.success('知识库已删除');
          if (currentCollection?.id === id) {
            setCurrentCollection(null);
            setView('collections');
          }
          loadCollections();
        } catch {
          message.error('删除失败');
        }
      },
    });
  };

  const handleUpload = async () => {
    if (!currentCollection) return;
    if (fileList.length === 0) {
      message.error('请选择文件');
      return;
    }
    try {
      for (const f of fileList) {
        await knowledgeService.uploadDocument(currentCollection.id, f.originFileObj);
      }
      message.success(`${fileList.length} 个文件上传成功，正在处理中...`);
      setUploadModalOpen(false);
      setFileList([]);
      loadDocuments(currentCollection.id);
    } catch {
      message.error('上传失败');
    }
  };

  const handleDeleteDocument = async (doc: KBDocument) => {
    Modal.confirm({
      title: '确认删除文档',
      content: `删除文档「${doc.file_name}」及其所有向量数据。`,
      okType: 'danger',
      onOk: async () => {
        try {
          await knowledgeService.deleteDocument(doc.id);
          message.success('文档已删除');
          if (currentCollection) loadDocuments(currentCollection.id);
        } catch {
          message.error('删除失败');
        }
      },
    });
  };

  const handleSearch = async () => {
    if (!currentCollection || !searchQuery.trim()) return;
    setLoading(true);
    try {
      const results = await knowledgeService.search(currentCollection.id, {
        query: searchQuery.trim(),
        top_k: 10,
      });
      setSearchResults(results);
      setView('search');
    } catch {
      setSearchResults([]);
      message.error('检索失败');
    } finally {
      setLoading(false);
    }
  };

  const getStatusTag = (status: string) => {
    const map: Record<string, { color: string; text: string }> = {
      processing: { color: 'blue', text: '处理中' },
      ready: { color: 'green', text: '就绪' },
      failed: { color: 'red', text: '失败' },
    };
    const item = map[status] || { color: 'default', text: status };
    return <Tag color={item.color}>{item.text}</Tag>;
  };

  const documentColumns = [
    { title: '文件名', dataIndex: 'file_name', key: 'file_name', ellipsis: true },
    { title: '类型', dataIndex: 'mime_type', key: 'mime_type', width: 120 },
    {
      title: '大小',
      dataIndex: 'file_size',
      key: 'file_size',
      width: 100,
      render: (v: number) => `${(v / 1024).toFixed(1)} KB`,
    },
    { title: '状态', dataIndex: 'status', key: 'status', width: 100, render: getStatusTag },
    { title: '分块数', dataIndex: 'chunk_count', key: 'chunk_count', width: 80 },
    {
      title: '操作',
      key: 'action',
      width: 80,
      render: (_: any, record: KBDocument) => (
        <Button
          type="link"
          danger
          size="small"
          icon={<DeleteOutlined />}
          onClick={() => handleDeleteDocument(record)}
        />
      ),
    },
  ];

  return (
    <div style={{ padding: 24 }}>
      <div style={{ marginBottom: 24, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <div>
          <Title level={3} style={{ margin: 0 }}>
            <DatabaseOutlined style={{ marginRight: 8 }} />
            知识库管理
          </Title>
          <Text type="secondary">文档向量化存储与智能检索</Text>
        </div>
        {view === 'collections' && (
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalOpen(true)}>
            新建知识库
          </Button>
        )}
        {view !== 'collections' && (
          <Button onClick={() => { setView('collections'); setCurrentCollection(null); }}>
            返回知识库列表
          </Button>
        )}
      </div>

      {view === 'collections' && (
        <Row gutter={[16, 16]}>
          {loading && collections.length === 0 ? (
            <Col span={24}>
              <Card><Empty description="加载中..." /></Card>
            </Col>
          ) : collections.length === 0 ? (
            <Col span={24}>
              <Card>
                <Empty
                  image={<InboxOutlined style={{ fontSize: 48, color: '#1677ff33' }} />}
                  description={
                    <span>
                      <Text type="secondary">暂无知识库</Text>
                    </span>
                  }
                >
                  <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalOpen(true)}>
                    创建第一个知识库
                  </Button>
                </Empty>
              </Card>
            </Col>
          ) : (
            collections.map((coll, idx) => {
              const gradient = COLLECTION_GRADIENTS[idx % COLLECTION_GRADIENTS.length];
              return (
                <Col xs={24} sm={12} md={8} lg={6} key={coll.id}>
                  <Card
                    hoverable
                    onClick={() => { setCurrentCollection(coll); setView('documents'); }}
                    className="knowledge-coll-card"
                    style={{ height: 160, cursor: 'pointer', overflow: 'hidden' }}
                    styles={{ body: { padding: 16, height: '100%', display: 'flex', flexDirection: 'column', justifyContent: 'space-between' } }}
                  >
                    <div className="knowledge-coll-top-bar" style={{ background: gradient }} />
                    <Space align="start" size={10}>
                      <div className="knowledge-coll-icon" style={{ background: gradient }}>
                        <DatabaseOutlined />
                      </div>
                      <div style={{ minWidth: 0 }}>
                        <Text strong style={{ fontSize: 15, display: 'block', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                          {coll.name}
                        </Text>
                        <Text type="secondary" style={{ fontSize: 12, display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden' }}>
                          {coll.description || '暂无描述'}
                        </Text>
                      </div>
                    </Space>
                    <Space size={4} style={{ justifyContent: 'space-between', width: '100%' }}>
                      <Tag color={coll.status === 'active' ? 'green' : 'default'}>
                        {coll.status === 'active' ? '活跃' : '停用'}
                      </Tag>
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        {new Date(coll.updated_at).toLocaleDateString()}
                      </Text>
                    </Space>
                    <Button
                      type="text"
                      size="small"
                      danger
                      icon={<DeleteOutlined />}
                      onClick={(e) => { e.stopPropagation(); handleDeleteCollection(coll.id); }}
                      style={{ position: 'absolute', top: 8, right: 8 }}
                    />
                  </Card>
                </Col>
              );
            })
          )}
        </Row>
      )}

      {view === 'documents' && currentCollection && (
        <Space direction="vertical" style={{ width: '100%' }} size="large">
          <Card>
            <Space direction="vertical" style={{ width: '100%' }} size="middle">
              <Title level={4} style={{ margin: 0 }}>{currentCollection.name}</Title>
              <Space>
                <Input
                  placeholder="输入自然语言检索知识库..."
                  prefix={<SearchOutlined />}
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  onPressEnter={handleSearch}
                  style={{ width: 400 }}
                />
                <Button type="primary" onClick={handleSearch}>
                  检索
                </Button>
                <Button icon={<UploadOutlined />} onClick={() => setUploadModalOpen(true)}>
                  上传文档
                </Button>
              </Space>
            </Space>
          </Card>

          <Card title="文档列表" size="small">
            <Table
              rowKey="id"
              loading={loading}
              columns={documentColumns}
              dataSource={documents}
              locale={{ emptyText: '暂无文档，上传文档开始构建知识库' }}
              pagination={{ pageSize: 10 }}
            />
          </Card>
        </Space>
      )}

      {view === 'search' && currentCollection && (
        <Space direction="vertical" style={{ width: '100%' }} size="large">
          <Card>
            <Space direction="vertical" style={{ width: '100%' }} size="middle">
              <Title level={4} style={{ margin: 0 }}>检索结果</Title>
              <Text type="secondary">查询: "{searchQuery}" · 共找到 {searchResults.length} 条结果</Text>
            </Space>
          </Card>

          {searchResults.length === 0 ? (
            <Card><Empty description="未找到相关内容" /></Card>
          ) : (
            searchResults.map((r, idx) => (
              <Card key={idx} size="small" className="fade-in">
                <Space direction="vertical" size={8} style={{ width: '100%' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', width: '100%' }}>
                    <Space wrap>
                      <Tag color="blue">{r.file_name}</Tag>
                      <Tag>块 #{r.chunk_index}</Tag>
                    </Space>
                    <Text strong style={{ fontSize: 13, color: r.score >= 0.7 ? '#10B981' : r.score >= 0.4 ? '#F59E0B' : '#EF4444' }}>
                      {(r.score * 100).toFixed(1)}% 相似度
                    </Text>
                  </div>
                  <div style={{ height: 5, background: 'var(--neutral-100)', borderRadius: 3, overflow: 'hidden', width: '100%' }}>
                    <div
                      style={{
                        height: '100%',
                        width: `${Math.min(100, r.score * 100)}%`,
                        background: r.score >= 0.7 ? 'linear-gradient(90deg, #10B981, #34D399)' : r.score >= 0.4 ? 'linear-gradient(90deg, #F59E0B, #FBBF24)' : 'linear-gradient(90deg, #EF4444, #F87171)',
                        borderRadius: 3,
                        transition: 'width 0.8s ease',
                      }}
                    />
                  </div>
                  <Text style={{ fontSize: 13, lineHeight: 1.7 }}>{r.content}</Text>
                </Space>
              </Card>
            ))
          )}

          <Button onClick={() => setView('documents')}>返回文档列表</Button>
        </Space>
      )}

      <Modal
        title="新建知识库"
        open={createModalOpen}
        onOk={handleCreateCollection}
        onCancel={() => { setCreateModalOpen(false); setCreateName(''); setCreateDesc(''); }}
        okText="创建"
      >
        <Space direction="vertical" style={{ width: '100%' }} size="middle">
          <Input
            placeholder="知识库名称"
            value={createName}
            onChange={(e) => setCreateName(e.target.value)}
          />
          <Input.TextArea
            placeholder="描述（可选）"
            value={createDesc}
            onChange={(e) => setCreateDesc(e.target.value)}
            rows={3}
          />
        </Space>
      </Modal>

      <Modal
        title={`上传文档到「${currentCollection?.name}」`}
        open={uploadModalOpen}
        onOk={handleUpload}
        onCancel={() => { setUploadModalOpen(false); setFileList([]); }}
        okText="上传"
      >
        <Upload
          multiple
          beforeUpload={() => false}
          fileList={fileList}
          onChange={({ fileList: fl }) => setFileList(fl)}
          accept=".txt,.md,.json,.csv,.html,.pdf,.docx"
        >
          <Button icon={<UploadOutlined />}>选择文件</Button>
        </Upload>
        <Text type="secondary" style={{ fontSize: 12 }}>
          支持 TXT、Markdown、JSON、CSV、HTML 等文本格式，上传后自动分块和向量化。
        </Text>
      </Modal>
    </div>
  );
}