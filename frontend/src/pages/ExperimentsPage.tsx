import { useCallback, useEffect, useState } from 'react';
import {
  Button, Card, Form, Input, InputNumber, Modal, Space, Table, Tabs, Tag, Typography, message,
} from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import {
  listCanaryReleases, createCanaryRelease, advanceCanary, promoteCanary, rollbackCanary, checkCanary,
  listShadowRules, createShadowRule, stopShadowRule, listShadowExecutions,
  createReplay, getReplay,
} from '../services/api';
import StatusBadge from '../components/StatusBadge';
import PayloadViewer from '../components/PayloadViewer';

const { Title, Text } = Typography;

// ── 模型(M5-C 契约) ──

interface CanaryRelease {
  id: string;
  business_app_code: string;
  graph_key: string;
  candidate_graph_key: string;
  stages: string;
  current_stage_index: number;
  max_error_rate: number;
  min_sample_size: number;
  status: string;
  created_at: string;
  updated_at: string;
}

interface ShadowRule {
  id: string;
  business_app_code: string;
  graph_key: string;
  shadow_graph_key: string;
  traffic_percent: number;
  status: string;
  created_at: string;
}

interface ShadowExecution {
  id: string;
  rule_id: string;
  primary_run_id: string;
  shadow_run_id: string;
  comparison_json?: string;
  created_at: string;
}

interface ReplaySession {
  id: string;
  source_run_id: string;
  replay_run_id?: string;
  graph_key: string;
  status: string;
  diff_json?: string;
  created_at: string;
}

interface CanaryCheckResult {
  release_id: string;
  candidate_graph_key: string;
  sample_size: number;
  failed_runs: number;
  error_rate: number;
  max_error_rate: number;
  rolled_back: boolean;
}

export default function ExperimentsPage() {
  // ── Canary ──
  const [canaries, setCanaries] = useState<CanaryRelease[]>([]);
  const [canaryLoading, setCanaryLoading] = useState(false);
  const [canaryModalOpen, setCanaryModalOpen] = useState(false);
  const [canaryForm] = Form.useForm();
  const [checkResult, setCheckResult] = useState<CanaryCheckResult | null>(null);

  // ── Shadow ──
  const [shadowRules, setShadowRules] = useState<ShadowRule[]>([]);
  const [shadowExecs, setShadowExecs] = useState<ShadowExecution[]>([]);
  const [shadowLoading, setShadowLoading] = useState(false);
  const [shadowModalOpen, setShadowModalOpen] = useState(false);
  const [shadowForm] = Form.useForm();

  // ── Replay ──
  const [replaySession, setReplaySession] = useState<ReplaySession | null>(null);
  const [replayLoading, setReplayLoading] = useState(false);
  const [replayForm] = Form.useForm();

  const loadCanaries = useCallback(async () => {
    setCanaryLoading(true);
    try {
      const { data } = await listCanaryReleases();
      setCanaries(data.data.items || []);
    } finally {
      setCanaryLoading(false);
    }
  }, []);

  const loadShadow = useCallback(async () => {
    setShadowLoading(true);
    try {
      const [rules, execs] = await Promise.all([listShadowRules(), listShadowExecutions()]);
      setShadowRules(rules.data.data.items || []);
      setShadowExecs(execs.data.data.items || []);
    } finally {
      setShadowLoading(false);
    }
  }, []);

  useEffect(() => {
    loadCanaries();
    loadShadow();
  }, [loadCanaries, loadShadow]);

  // ── Canary 操作 ──
  const handleCreateCanary = async () => {
    const values = await canaryForm.validateFields();
    await createCanaryRelease({
      ...values,
      stages: values.stages.split(',').map((s: string) => parseInt(s.trim(), 10)).filter((n: number) => !Number.isNaN(n)),
    });
    message.success('Canary 发布已创建');
    setCanaryModalOpen(false);
    canaryForm.resetFields();
    loadCanaries();
  };

  const canaryAction = async (id: string, action: 'advance' | 'promote' | 'rollback' | 'check') => {
    const { data } = await (
      action === 'advance' ? advanceCanary(id)
        : action === 'promote' ? promoteCanary(id)
          : action === 'rollback' ? rollbackCanary(id)
            : checkCanary(id)
    );
    message.success(`${action} 完成`);
    if (action === 'check') setCheckResult(data.data);
    loadCanaries();
  };

  const currentStage = (c: CanaryRelease): number => {
    try {
      const stages = JSON.parse(c.stages);
      return stages[c.current_stage_index] ?? 0;
    } catch {
      return 0;
    }
  };

  const canaryColumns = [
    { title: '发布 ID', dataIndex: 'id', width: 130, render: (v: string) => <Text copyable style={{ fontSize: 12 }}>{v.slice(0, 8)}…</Text> },
    { title: '业务域', dataIndex: 'business_app_code', width: 100 },
    { title: '基线 Graph', dataIndex: 'graph_key', width: 150 },
    { title: '候选 Graph', dataIndex: 'candidate_graph_key', width: 150, render: (v: string) => <Tag color="orange">{v}</Tag> },
    { title: '当前流量', width: 100, render: (_: unknown, c: CanaryRelease) => `${currentStage(c)}%` },
    { title: '状态', dataIndex: 'status', width: 110, render: (v: string) => <StatusBadge status={v} /> },
    {
      title: '操作',
      width: 280,
      render: (_: unknown, c: CanaryRelease) =>
        c.status === 'active' ? (
          <Space>
            <Button size="small" onClick={() => canaryAction(c.id, 'check')}>健康检查</Button>
            <Button size="small" onClick={() => canaryAction(c.id, 'advance')}>推进</Button>
            <Button size="small" type="primary" ghost onClick={() => canaryAction(c.id, 'promote')}>全量</Button>
            <Button size="small" danger onClick={() => canaryAction(c.id, 'rollback')}>回滚</Button>
          </Space>
        ) : '—',
    },
  ];

  // ── Shadow 操作 ──
  const handleCreateShadow = async () => {
    const values = await shadowForm.validateFields();
    await createShadowRule(values);
    message.success('Shadow 规则已创建');
    setShadowModalOpen(false);
    shadowForm.resetFields();
    loadShadow();
  };

  const shadowColumns = [
    { title: '规则 ID', dataIndex: 'id', width: 130, render: (v: string) => <Text copyable style={{ fontSize: 12 }}>{v.slice(0, 8)}…</Text> },
    { title: '业务域', dataIndex: 'business_app_code', width: 100 },
    { title: '基线 Graph', dataIndex: 'graph_key', width: 160 },
    { title: '影子 Graph', dataIndex: 'shadow_graph_key', width: 160, render: (v: string) => <Tag color="purple">{v}</Tag> },
    { title: '流量比例', dataIndex: 'traffic_percent', width: 100, render: (v: number) => `${v}%` },
    { title: '状态', dataIndex: 'status', width: 100, render: (v: string) => <StatusBadge status={v} /> },
    {
      title: '操作',
      width: 100,
      render: (_: unknown, r: ShadowRule) =>
        r.status === 'active' ? (
          <Button size="small" danger onClick={() => stopShadowRule(r.id).then(() => { message.success('已停止'); loadShadow(); })}>停止</Button>
        ) : '—',
    },
  ];

  const execColumns = [
    { title: '主 Run', dataIndex: 'primary_run_id', width: 150, render: (v: string) => <Text style={{ fontSize: 12 }}>{v.slice(0, 8)}…</Text> },
    { title: '影子 Run', dataIndex: 'shadow_run_id', width: 150, render: (v: string) => <Text style={{ fontSize: 12 }}>{v.slice(0, 8)}…</Text> },
    { title: '对比结果', dataIndex: 'comparison_json', width: 150, render: (v?: string) => <PayloadViewer value={v} /> },
    { title: '时间', dataIndex: 'created_at', width: 170, render: (v: string) => new Date(v).toLocaleString() },
  ];

  // ── Replay 操作 ──
  const handleReplay = async () => {
    const values = await replayForm.validateFields();
    setReplayLoading(true);
    try {
      const { data } = await createReplay(values);
      message.success('重放已提交');
      const session = await getReplay(data.data.id);
      setReplaySession(session.data.data);
    } finally {
      setReplayLoading(false);
    }
  };

  const tabs = [
    {
      key: 'canary',
      label: 'Canary 发布',
      children: (
        <Card size="small">
          <Space style={{ marginBottom: 12 }}>
            <Button type="primary" onClick={() => setCanaryModalOpen(true)}>创建发布</Button>
            <Button icon={<ReloadOutlined />} onClick={loadCanaries}>刷新</Button>
          </Space>
          {checkResult && (
            <Card size="small" type="inner" title="最近健康检查" style={{ marginBottom: 12 }}>
              <Space wrap>
                <Text>样本: {checkResult.sample_size}</Text>
                <Text>失败: {checkResult.failed_runs}</Text>
                <Text>错误率: {(checkResult.error_rate * 100).toFixed(1)}% (阈值 {(checkResult.max_error_rate * 100).toFixed(0)}%)</Text>
                {checkResult.rolled_back ? <Tag color="error">已自动回滚</Tag> : <Tag color="success">正常</Tag>}
              </Space>
            </Card>
          )}
          <Table rowKey="id" size="small" loading={canaryLoading} dataSource={canaries} columns={canaryColumns} pagination={{ pageSize: 10, showSizeChanger: false }} scroll={{ x: 1000 }} />
        </Card>
      ),
    },
    {
      key: 'shadow',
      label: 'Shadow 流量',
      children: (
        <Space direction="vertical" style={{ width: '100%' }} size="middle">
          <Card size="small" title="规则">
            <Space style={{ marginBottom: 12 }}>
              <Button type="primary" onClick={() => setShadowModalOpen(true)}>创建规则</Button>
              <Button icon={<ReloadOutlined />} onClick={loadShadow}>刷新</Button>
            </Space>
            <Table rowKey="id" size="small" loading={shadowLoading} dataSource={shadowRules} columns={shadowColumns} pagination={{ pageSize: 10, showSizeChanger: false }} scroll={{ x: 900 }} />
          </Card>
          <Card size="small" title="影子执行记录">
            <Table rowKey="id" size="small" dataSource={shadowExecs} columns={execColumns} pagination={{ pageSize: 10, showSizeChanger: false }} />
          </Card>
        </Space>
      ),
    },
    {
      key: 'replay',
      label: 'Replay 重放',
      children: (
        <Card size="small">
          <Form form={replayForm} layout="inline" onFinish={handleReplay}>
            <Form.Item name="source_run_id" rules={[{ required: true, message: '请输入源 Run ID' }]}>
              <Input placeholder="源 Run ID" style={{ width: 320 }} />
            </Form.Item>
            <Form.Item name="graph_key">
              <Input placeholder="目标 Graph Key(可选,缺省用源)" style={{ width: 220 }} />
            </Form.Item>
            <Form.Item>
              <Button type="primary" htmlType="submit" loading={replayLoading}>发起重放</Button>
            </Form.Item>
          </Form>
          {replaySession && (
            <Card size="small" type="inner" title="重放会话" style={{ marginTop: 16 }}>
              <Space direction="vertical" style={{ width: '100%' }}>
                <Space wrap>
                  <Text>会话: {replaySession.id.slice(0, 8)}…</Text>
                  <StatusBadge status={replaySession.status} />
                  <Text>Graph: {replaySession.graph_key}</Text>
                  {replaySession.replay_run_id && <Text>重放 Run: {replaySession.replay_run_id.slice(0, 8)}…</Text>}
                </Space>
                <div>
                  <Text type="secondary">Diff</Text>
                  <PayloadViewer value={replaySession.diff_json} />
                </div>
              </Space>
            </Card>
          )}
        </Card>
      ),
    },
  ];

  return (
    <div>
      <Title level={4}>实验中心 · 受控路由</Title>
      <Tabs items={tabs} />

      {/* Canary 创建 */}
      <Modal
        title="创建 Canary 发布"
        open={canaryModalOpen}
        onOk={handleCreateCanary}
        onCancel={() => setCanaryModalOpen(false)}
        okText="创建"
      >
        <Form form={canaryForm} layout="vertical">
          <Form.Item name="business_app_code" label="业务域" rules={[{ required: true }]}>
            <Input placeholder="finance" />
          </Form.Item>
          <Form.Item name="graph_key" label="基线 Graph Key" rules={[{ required: true }]}>
            <Input placeholder="finance_report_v1" />
          </Form.Item>
          <Form.Item name="candidate_graph_key" label="候选 Graph Key" rules={[{ required: true }]}>
            <Input placeholder="finance_report_v2" />
          </Form.Item>
          <Form.Item name="stages" label="流量阶段(逗号分隔百分比)" rules={[{ required: true }]} initialValue="5,25,50,100">
            <Input placeholder="5,25,50,100" />
          </Form.Item>
          <Space>
            <Form.Item name="max_error_rate" label="最大错误率(0-1)" rules={[{ required: true }]} initialValue={0.1}>
              <InputNumber min={0.01} max={1} step={0.05} />
            </Form.Item>
            <Form.Item name="min_sample_size" label="最小样本数" rules={[{ required: true }]} initialValue={10}>
              <InputNumber min={1} />
            </Form.Item>
          </Space>
        </Form>
      </Modal>

      {/* Shadow 创建 */}
      <Modal
        title="创建 Shadow 规则"
        open={shadowModalOpen}
        onOk={handleCreateShadow}
        onCancel={() => setShadowModalOpen(false)}
        okText="创建"
      >
        <Form form={shadowForm} layout="vertical">
          <Form.Item name="business_app_code" label="业务域" rules={[{ required: true }]}>
            <Input placeholder="finance" />
          </Form.Item>
          <Form.Item name="graph_key" label="基线 Graph Key" rules={[{ required: true }]}>
            <Input placeholder="finance_report_v1" />
          </Form.Item>
          <Form.Item name="shadow_graph_key" label="影子 Graph Key" rules={[{ required: true }]}>
            <Input placeholder="finance_report_v2" />
          </Form.Item>
          <Form.Item name="traffic_percent" label="流量比例(%)" rules={[{ required: true }]} initialValue={10}>
            <InputNumber min={0} max={100} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
