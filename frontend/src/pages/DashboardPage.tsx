import { useEffect, useState, useCallback, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Alert, Badge, Card, Col, Empty, List, Row, Segmented, Space, Spin, Statistic, Tag, Tooltip, Typography, Button,
} from 'antd';
import {
  ReloadOutlined, RobotOutlined, MessageOutlined, ThunderboltOutlined,
  CheckCircleOutlined, WarningOutlined,
} from '@ant-design/icons';
import {
  getBusinessApps, getApprovalTasks, getRuns, generateEvalReport,
  getDeadLetterToolCalls, getOutboxEntries,
} from '../services/api';
import { getDashboard, type DashboardResponse, type DashboardSummary } from '../services/dashboard';
import StatusBadge from '../components/StatusBadge';
import { useAuthStore } from '../store/auth';
import { safeRequest } from '../utils/errorHandler';
import Sparkline from '../components/charts/Sparkline';
import DonutChart from '../components/charts/DonutChart';
import HBarChart from '../components/charts/HBarChart';

const { Title, Text } = Typography;

const appNameMap: Record<string, string> = { finance: '财务中心' };
const appDescriptionMap: Record<string, string> = {
  finance: '上传财务数据，运行智能体分析流程，生成运营报告并完成审批归档。',
};

interface BusinessApp { code: string; name: string; description: string }
interface RunItem { id: string; status: string; graph_key: string; updated_at: string }
interface EvalSummary { total_tokens?: number; total_cost?: number; avg_duration?: number; error_rate?: number }

const SAMPLE_APPS: BusinessApp[] = [
  { code: 'finance', name: '财务中心', description: '上传财务数据，运行智能体分析流程，生成运营报告并完成审批归档。' },
];

const SAMPLE_RUNS: RunItem[] = [
  { id: 'run-1', status: 'running', graph_key: 'invoice_analysis', updated_at: new Date().toISOString() },
  { id: 'run-2', status: 'success', graph_key: 'expense_review', updated_at: new Date(Date.now() - 3600000).toISOString() },
  { id: 'run-3', status: 'pending_approval', graph_key: 'budget_approval', updated_at: new Date(Date.now() - 7200000).toISOString() },
];

const SAMPLE_SUMMARY: DashboardSummary = {
  total_agents: 6,
  total_conversations: 128,
  total_runs_7d: 356,
  total_cost_7d: 12.5,
  avg_success_rate: 87.5,
  active_users_7d: 18,
  top_departments: [
    { name: '财务部', efficiency_score: 92 },
    { name: '审计部', efficiency_score: 78 },
    { name: '运营部', efficiency_score: 65 },
    { name: '人事部', efficiency_score: 54 },
  ],
};

function usePermissionCards() {
  const hasPermission = useAuthStore((s) => s.hasPermission);
  // useMemo 保证 cards 引用稳定，否则每次渲染返回新对象，
  // 会导致依赖它的 loadDashboard/useEffect 无限循环触发请求（引发 429）
  return useMemo(() => ({
    approvals: hasPermission('approval:read'),
    runs: hasPermission('workflow:read'),
    eval: hasPermission('eval:read'),
    reliability: hasPermission('tool:read') || hasPermission('outbox:read'),
    experiments: hasPermission('experiment:manage'),
    toolRead: hasPermission('tool:read'),
    outboxRead: hasPermission('outbox:read'),
    agentRead: hasPermission('agent:read'),
  }), [hasPermission]);
}

interface StatCardProps {
  title: string;
  value: number | string;
  suffix?: string;
  precision?: number;
  icon: React.ReactNode;
  gradient: string;
  trend?: number[];
  valueColor?: string;
  onClick?: () => void;
}

function StatCard({
  title, value, suffix, precision, icon, gradient, trend, valueColor, onClick,
}: StatCardProps) {
  return (
    <Card
      className="dashboard-stat-card"
      style={{ height: '100%', cursor: onClick ? 'pointer' : 'default' }}
      styles={{ body: { padding: 16 } }}
      onClick={onClick}
    >
      <Space align="center" size={12} style={{ width: '100%', justifyContent: 'space-between' }}>
        <Space align="center" size={12}>
          <div className="dashboard-stat-icon" style={{ background: gradient }}>{icon}</div>
          <div>
            <Text type="secondary" style={{ fontSize: 13, display: 'block', lineHeight: 1.4 }}>{title}</Text>
            <Statistic
              value={value}
              suffix={suffix}
              precision={precision}
              valueStyle={{ fontSize: 26, fontWeight: 700, lineHeight: 1.2, color: valueColor }}
            />
          </div>
        </Space>
        {trend && trend.length > 1 && (
          <Sparkline data={trend} width={72} height={32} stroke={gradient.split(',')[0].replace('linear-gradient(135deg, ', '').trim()} />
        )}
      </Space>
    </Card>
  );
}

export default function DashboardPage() {
  const navigate = useNavigate();
  const cards = usePermissionCards();

  const [apps, setApps] = useState<BusinessApp[]>([]);
  const [pendingApprovals, setPendingApprovals] = useState<number | null>(null);
  const [activeRuns, setActiveRuns] = useState<RunItem[]>([]);
  const [evalSummary, setEvalSummary] = useState<EvalSummary | null>(null);
  const [dlqCount, setDlqCount] = useState<number | null>(null);
  const [outboxPending, setOutboxPending] = useState<number | null>(null);
  const [dash, setDash] = useState<DashboardResponse | null>(null);
  const [period, setPeriod] = useState<number>(7);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);

  const loadDashboard = useCallback(async () => {
    setLoading(true);
    setLoadError(false);
    const today = new Date();
    const startISO = new Date(today.getFullYear(), today.getMonth(), today.getDate()).toISOString();
    const endISO = new Date(today.getFullYear(), today.getMonth(), today.getDate() + 1).toISOString();

    try {
      const appsData = await safeRequest(
        () => getBusinessApps().then(({ data }) => data.data || data),
        SAMPLE_APPS,
        { showErrorMessage: false }
      );
      setApps(Array.isArray(appsData) ? appsData : SAMPLE_APPS);

      if (cards.approvals) {
        const result = await safeRequest(
          () => getApprovalTasks({ status: 'pending', page: '1', page_size: '1' })
            .then(({ data }) => data.pagination?.total ?? (data.data || []).length),
          0,
          { showErrorMessage: false }
        );
        setPendingApprovals(result);
      } else {
        setPendingApprovals(null);
      }

      if (cards.runs) {
        const result = await safeRequest(
          () => getRuns({ limit: '8' }).then(({ data }) => data.data?.items || data.items || []),
          SAMPLE_RUNS,
          { showErrorMessage: false }
        );
        setActiveRuns(Array.isArray(result) && result.length > 0 ? result : SAMPLE_RUNS);
      } else {
        setActiveRuns([]);
      }

      if (cards.eval) {
        const result = await safeRequest(
          () => generateEvalReport({ start_time: startISO, end_time: endISO })
            .then(({ data }) => data.data?.summary || data.summary || null),
          null,
          { showErrorMessage: false }
        );
        setEvalSummary(result);
      } else {
        setEvalSummary(null);
      }

      if (cards.toolRead) {
        const result = await safeRequest(
          () => getDeadLetterToolCalls({ limit: '100' }).then(({ data }) => (data.data?.items || data.items || []).length),
          0,
          { showErrorMessage: false }
        );
        setDlqCount(result);
      } else {
        setDlqCount(null);
      }

      if (cards.outboxRead) {
        const result = await safeRequest(
          () => getOutboxEntries({ limit: '100' }).then(({ data }) => {
            const items = data.data?.items || data.items || [];
            return items.filter((e: { state: string }) => !['delivered', 'compensated'].includes(e.state)).length;
          }),
          0,
          { showErrorMessage: false }
        );
        setOutboxPending(result);
      } else {
        setOutboxPending(null);
      }

      if (cards.agentRead) {
        const dashData = await safeRequest(
          () => getDashboard({ days: period }).then((d) => d || null),
          null,
          { showErrorMessage: false }
        );
        setDash(dashData);
      } else {
        setDash(null);
      }
    } catch {
      setLoadError(true);
    } finally {
      setLoading(false);
    }
  }, [cards, period]);

  useEffect(() => {
    loadDashboard();
  }, [loadDashboard]);

  if (loading) return <Spin size="large" style={{ display: 'block', margin: '100px auto' }} />;

  const summary = dash?.summary ?? SAMPLE_SUMMARY;
  const agentMetrics = dash?.agent_metrics ?? [];
  const failureReasons = dash?.failure_reasons ?? [];
  const hasDashData = Boolean(dash);
  const agentTrend = agentMetrics.length > 0
    ? agentMetrics[0].trend_7d.map((t) => t.runs)
    : [0, 0, 0, 0, 0, 0, 0];

  return (
    <div className="fade-in">
      <Space style={{ marginBottom: 16, justifyContent: 'space-between', width: '100%', flexWrap: 'wrap' }}>
        <div>
          <Title level={4} style={{ margin: 0 }}>工作台</Title>
          <Text type="secondary" style={{ fontSize: 13 }}>
            {new Date().toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric', weekday: 'long' })}
          </Text>
        </div>
        <Space>
          <Segmented
            value={period}
            options={[{ label: '近 7 天', value: 7 }, { label: '近 30 天', value: 30 }]}
            onChange={(v) => setPeriod(v as number)}
          />
          <Button icon={<ReloadOutlined />} onClick={() => loadDashboard()}>刷新</Button>
        </Space>
      </Space>

      {loadError && (
        <Alert
          style={{ marginBottom: 16 }}
          type="warning"
          showIcon
          message="部分数据加载失败"
          description='系统正在使用示例数据展示，点击"刷新"可尝试获取最新数据。'
        />
      )}

      {/* 统计卡片 */}
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} lg={6}>
          <StatCard
            title="智能体总数"
            value={summary.total_agents}
            icon={<RobotOutlined />}
            gradient="linear-gradient(135deg, #667EEA 0%, #764BA2 100%)"
            trend={agentTrend}
          />
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <StatCard
            title="累计对话"
            value={summary.total_conversations}
            icon={<MessageOutlined />}
            gradient="linear-gradient(135deg, #4FACFE 0%, #00F2FE 100%)"
          />
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <StatCard
            title={`近 ${period} 天执行`}
            value={summary.total_runs_7d}
            icon={<ThunderboltOutlined />}
            gradient="linear-gradient(135deg, #11998E 0%, #38EF7D 100%)"
          />
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card className="dashboard-stat-card" style={{ height: '100%' }} styles={{ body: { padding: 16 } }}>
            <Space align="center" size={12} style={{ width: '100%', justifyContent: 'space-between' }}>
              <Space align="center" size={12}>
                <div className="dashboard-stat-icon" style={{ background: 'linear-gradient(135deg, #F6D365 0%, #FDA085 100%)' }}>
                  <CheckCircleOutlined />
                </div>
                <div>
                  <Text type="secondary" style={{ fontSize: 13, display: 'block', lineHeight: 1.4 }}>平均成功率</Text>
                  <Statistic
                    value={summary.avg_success_rate}
                    suffix="%"
                    precision={1}
                    valueStyle={{ fontSize: 26, fontWeight: 700, lineHeight: 1.2, color: '#10B981' }}
                  />
                </div>
              </Space>
              <DonutChart percent={summary.avg_success_rate} size={56} strokeWidth={7} color="#10B981" />
            </Space>
          </Card>
        </Col>
      </Row>

      {/* 图表可视化 */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={8}>
          <Card
            className="dashboard-chart-card"
            title="部门效率"
            extra={hasDashData ? <Text type="secondary" style={{ fontSize: 12 }}>效率评分</Text> : undefined}
            style={{ height: '100%' }}
          >
            {summary.top_departments.length === 0 ? (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无数据" />
            ) : (
              <HBarChart
                data={summary.top_departments.map((d, i) => ({
                  label: d.name,
                  value: d.efficiency_score,
                  color: `linear-gradient(90deg, ${['#667EEA', '#11998E', '#F6D365', '#4FACFE'][i % 4]} 0%, ${['#764BA2', '#38EF7D', '#FDA085', '#00F2FE'][i % 4]} 100%)`,
                }))}
              />
            )}
          </Card>
        </Col>
        <Col xs={24} lg={8}>
          <Card
            className="dashboard-chart-card"
            title="智能体质量趋势"
            extra={hasDashData ? <Text type="secondary" style={{ fontSize: 12 }}>近 7 日执行</Text> : undefined}
            style={{ height: '100%' }}
          >
            {agentMetrics.length === 0 ? (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无数据" />
            ) : (
              agentMetrics.slice(0, 5).map((m) => (
                <div key={m.agent_code} className="dashboard-agent-row">
                  <Tag color={m.success_rate >= 0.9 ? 'success' : m.success_rate >= 0.7 ? 'processing' : 'warning'} style={{ minWidth: 40, textAlign: 'center', margin: 0 }}>
                    {Math.round(m.success_rate * 100)}%
                  </Tag>
                  <Tooltip title={`${m.total_runs} 次执行`}>
                    <Text style={{ fontSize: 13, flex: 1, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                      {m.agent_name}
                    </Text>
                  </Tooltip>
                  <Sparkline data={m.trend_7d.map((t) => t.runs)} width={64} height={26} />
                </div>
              ))
            )}
          </Card>
        </Col>
        <Col xs={24} lg={8}>
          <Card
            className="dashboard-chart-card"
            title="失败原因分布"
            extra={hasDashData ? <Text type="secondary" style={{ fontSize: 12 }}>占比</Text> : undefined}
            style={{ height: '100%' }}
          >
            {failureReasons.length === 0 ? (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无数据" />
            ) : (
              failureReasons.slice(0, 6).map((r) => (
                <div key={r.category} style={{ marginBottom: 12 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 4 }}>
                    <Space size={6}>
                      <WarningOutlined style={{ fontSize: 12, color: r.severity === 'high' ? '#EF4444' : r.severity === 'medium' ? '#F59E0B' : '#9CA3AF' }} />
                      <Text style={{ fontSize: 13 }}>{r.category}</Text>
                    </Space>
                    <Text style={{ fontSize: 12, fontWeight: 600, color: 'var(--text-primary)' }}>
                      {r.percentage.toFixed(1)}%
                    </Text>
                  </div>
                  <div style={{ height: 6, background: 'var(--neutral-100)', borderRadius: 3, overflow: 'hidden' }}>
                    <div
                      style={{
                        height: '100%',
                        width: `${r.percentage}%`,
                        background: r.severity === 'high' ? 'linear-gradient(90deg, #EF4444, #F97316)'
                          : r.severity === 'medium' ? 'linear-gradient(90deg, #F59E0B, #FBBF24)'
                            : 'linear-gradient(90deg, #9CA3AF, #D1D5DB)',
                        borderRadius: 3,
                        transition: 'width 0.8s ease',
                      }}
                    />
                  </div>
                </div>
              ))
            )}
          </Card>
        </Col>
      </Row>

      {/* 第一行:待办审批 + 系统健康(进行中 Run 概览) */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={12}>
          <Card
            title="待办审批"
            hoverable={pendingApprovals != null && pendingApprovals > 0}
            onClick={() => pendingApprovals ? navigate('/explore/tool-calls', { state: { status: 'pending_approval' } }) : undefined}
          >
            {pendingApprovals == null ? (
              <Text type="secondary">无权限或加载失败</Text>
            ) : pendingApprovals === 0 ? (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无待审批任务" />
            ) : (
              <Statistic title="待处理 ToolCall 审批" value={pendingApprovals} suffix="项" valueStyle={{ color: '#faad14' }} />
            )}
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card title="进行中 Run">
            {activeRuns.length === 0 ? (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无 Run 记录" />
            ) : (
              <List
                size="small"
                dataSource={activeRuns.slice(0, 5)}
                renderItem={(r) => (
                  <List.Item
                    style={{ cursor: 'pointer', padding: '6px 0' }}
                    onClick={() => navigate(`/runs/${r.id}`)}
                  >
                    <Space>
                      <StatusBadge status={r.status} />
                      <Text style={{ fontSize: 12 }}>{r.graph_key}</Text>
                      <Text type="secondary" style={{ fontSize: 12 }}>{new Date(r.updated_at).toLocaleString()}</Text>
                    </Space>
                  </List.Item>
                )}
              />
            )}
          </Card>
        </Col>
      </Row>

      {/* 第二行:今日概览(Eval) + 可靠性 */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        {cards.eval && (
          <Col xs={24} lg={12}>
            <Card title="今日概览 (Eval)">
              {evalSummary == null ? (
                <Text type="secondary">暂无评估数据</Text>
              ) : (
                <Row gutter={12}>
                  <Col span={6}><Statistic title="Tokens" value={evalSummary.total_tokens ?? 0} /></Col>
                  <Col span={6}><Statistic title="成本" value={evalSummary.total_cost ?? 0} precision={4} /></Col>
                  <Col span={6}><Statistic title="平均时长(ms)" value={evalSummary.avg_duration ?? 0} /></Col>
                  <Col span={6}><Statistic title="错误率" value={(evalSummary.error_rate ?? 0) * 100} precision={1} suffix="%" valueStyle={{ color: (evalSummary.error_rate ?? 0) > 0.1 ? '#cf1322' : undefined }} /></Col>
                </Row>
              )}
            </Card>
          </Col>
        )}
        {cards.reliability && (
          <Col xs={24} lg={12}>
            <Card title="可靠性" hoverable onClick={() => navigate('/operations/outbox')}>
              <Space size="large">
                {dlqCount != null && (
                  <Badge count={dlqCount} offset={[12, 0]} size="small">
                    <Statistic title="死信队列" value={dlqCount} suffix="条" valueStyle={{ color: dlqCount > 0 ? '#cf1322' : undefined }} />
                  </Badge>
                )}
                {outboxPending != null && (
                  <Statistic title="Outbox 未完成" value={outboxPending} suffix="条" valueStyle={{ color: outboxPending > 0 ? '#faad14' : undefined }} />
                )}
                {dlqCount == null && outboxPending == null && <Text type="secondary">无权限或加载失败</Text>}
              </Space>
            </Card>
          </Col>
        )}
      </Row>

      {/* 实验提示 */}
      {cards.experiments && (
        <Alert
          style={{ marginTop: 16 }}
          type="info"
          showIcon
          message="受控路由"
          description={<>当前存在 Canary/Shadow 实验能力，前往 <a onClick={() => navigate('/experiments')}>实验中心</a> 管理流量切分。</>}
        />
      )}

      {/* 业务应用入口 */}
      <Title level={5} style={{ marginTop: 24 }}>业务应用</Title>
      <Row gutter={[16, 16]}>
        {apps.map((app) => (
          <Col key={app.code} xs={24} sm={12} lg={8}>
            <Card hoverable title={appNameMap[app.code] || app.name} onClick={() => navigate(`/${app.code}`)}>
              {appDescriptionMap[app.code] || app.description}
            </Card>
          </Col>
        ))}
      </Row>
    </div>
  );
}
