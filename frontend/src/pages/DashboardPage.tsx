import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Alert, Badge, Card, Col, Empty, List, Row, Space, Spin, Statistic, Typography,
} from 'antd';
import {
  getBusinessApps, getApprovalTasks, getRuns, generateEvalReport,
  getDeadLetterToolCalls, getOutboxEntries,
} from '../services/api';
import StatusBadge from '../components/StatusBadge';
import { useAuthStore } from '../store/auth';

const { Title, Text } = Typography;

const appNameMap: Record<string, string> = { finance: '财务中心' };
const appDescriptionMap: Record<string, string> = {
  finance: '上传财务数据，运行智能体分析流程，生成运营报告并完成审批归档。',
};

interface BusinessApp { code: string; name: string; description: string }
interface RunItem { id: string; status: string; graph_key: string; updated_at: string }
interface EvalSummary { total_tokens?: number; total_cost?: number; avg_duration?: number; error_rate?: number }

// 按权限渲染卡片:无权限或加载失败时显示占位(Spec §3.2 权限驱动)。
function usePermissionCards() {
  const hasPermission = useAuthStore((s) => s.hasPermission);
  return {
    approvals: hasPermission('approval:read'),
    runs: hasPermission('workflow:read'),
    eval: hasPermission('eval:read'),
    reliability: hasPermission('tool:read') || hasPermission('outbox:read'),
    experiments: hasPermission('experiment:manage'),
    toolRead: hasPermission('tool:read'),
    outboxRead: hasPermission('outbox:read'),
  };
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
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const today = new Date();
    const startISO = new Date(today.getFullYear(), today.getMonth(), today.getDate()).toISOString();
    const endISO = new Date(today.getFullYear(), today.getMonth(), today.getDate() + 1).toISOString();

    const jobs: Promise<unknown>[] = [getBusinessApps().then(({ data }) => setApps(data.data))];
    if (cards.approvals) {
      jobs.push(getApprovalTasks({ status: 'pending', page: '1', page_size: '1' })
        .then(({ data }) => setPendingApprovals(data.pagination?.total ?? (data.data || []).length))
        .catch(() => setPendingApprovals(null)));
    }
    if (cards.runs) {
      jobs.push(getRuns({ limit: '8' })
        .then(({ data }) => setActiveRuns(data.data.items || []))
        .catch(() => setActiveRuns([])));
    }
    if (cards.eval) {
      jobs.push(generateEvalReport({ start_time: startISO, end_time: endISO })
        .then(({ data }) => setEvalSummary(data.data.summary))
        .catch(() => setEvalSummary(null)));
    }
    if (cards.reliability) {
      if (cards.toolRead) {
        jobs.push(getDeadLetterToolCalls({ limit: '100' })
          .then(({ data }) => setDlqCount((data.data.items || []).length))
          .catch(() => setDlqCount(null)));
      }
      if (cards.outboxRead) {
        jobs.push(getOutboxEntries({ limit: '100' })
          .then(({ data }) => {
            const items = data.data.items || [];
            setOutboxPending(items.filter((e: { state: string }) => !['delivered', 'compensated'].includes(e.state)).length);
          })
          .catch(() => setOutboxPending(null)));
      }
    }
    Promise.all(jobs).finally(() => setLoading(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (loading) return <Spin size="large" style={{ display: 'block', margin: '100px auto' }} />;

  return (
    <div>
      <Title level={4}>工作台</Title>

      {/* 第一行:待办审批 + 系统健康(进行中 Run 概览) */}
      <Row gutter={[16, 16]}>
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
