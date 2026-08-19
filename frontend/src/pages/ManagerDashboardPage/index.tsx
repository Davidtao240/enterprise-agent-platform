import { useCallback, useEffect, useState } from 'react';
import {
  Row, Col, Card, Statistic, Spin, DatePicker, Typography, Progress,
  Table, Tag, Space, Select, Tooltip, List,
} from 'antd';
import {
  TeamOutlined, MessageOutlined, ThunderboltOutlined, DollarOutlined,
  CheckCircleOutlined, UserOutlined, TrophyOutlined, WarningOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import type { Dayjs } from 'dayjs';
import {
  getDashboard, DashboardResponse, DepartmentEfficiency,
  AgentQualityMetric,
} from '../../services/dashboard';
import { useAuthStore } from '../../store/auth';

const { Title, Text } = Typography;
const { RangePicker } = DatePicker;

type SortKey = 'savings' | 'success' | 'cost' | 'usage';

export default function ManagerDashboardPage() {
  const token = useAuthStore((s) => s.token);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<DashboardResponse | null>(null);
  const [range, setRange] = useState<[Dayjs, Dayjs] | null>(null);
  const [sortKey, setSortKey] = useState<SortKey>('savings');

  const loadData = useCallback(async () => {
    if (!token) return;
    setLoading(true);
    try {
      const params: { days?: number } = {};
      if (range) {
        const days = range[1].diff(range[0], 'day');
        params.days = Math.min(days, 90);
      }
      const response = await getDashboard(params);
      setData(response);
    } catch {
      setData(null);
    } finally {
      setLoading(false);
    }
  }, [token, range]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const departmentColumns: ColumnsType<DepartmentEfficiency> = [
    {
      title: '部门',
      dataIndex: 'department',
      key: 'department',
      render: (v: string) => <Text strong>{v}</Text>,
    },
    {
      title: 'Agent 数',
      dataIndex: 'agent_count',
      key: 'agent_count',
    },
    {
      title: '对话数',
      dataIndex: 'total_conversations',
      key: 'total_conversations',
      render: (v: number) => v.toLocaleString(),
    },
    {
      title: '节省工时(小时)',
      dataIndex: 'savings_estimate_hours',
      key: 'savings',
      render: (v: number) => (
        <Text strong style={{ color: '#52c41a' }}>
          <TrophyOutlined /> {v.toFixed(1)}h
        </Text>
      ),
      sorter: (a, b) => a.savings_estimate_hours - b.savings_estimate_hours,
      defaultSortOrder: sortKey === 'savings' ? 'descend' : undefined,
    },
    {
      title: '成本(USD)',
      dataIndex: 'cost_usd',
      key: 'cost',
      render: (v: number) => `$${v.toFixed(2)}`,
      sorter: (a, b) => a.cost_usd - b.cost_usd,
    },
    {
      title: '平均耗时',
      dataIndex: 'avg_duration_ms',
      key: 'avg_duration',
      render: (v: number) => `${(v / 1000 / 60).toFixed(1)}分钟`,
    },
  ];

  const agentColumns: ColumnsType<AgentQualityMetric> = [
    {
      title: 'Agent',
      dataIndex: 'agent_name',
      key: 'agent_name',
      render: (v: string, r: AgentQualityMetric) => (
        <div>
          <Text strong>{v}</Text>
          <br />
          <Tag>{r.department}</Tag>
        </div>
      ),
    },
    {
      title: '运行次数',
      dataIndex: 'total_runs',
      key: 'total_runs',
      render: (v: number) => v.toLocaleString(),
      sorter: (a, b) => a.total_runs - b.total_runs,
      defaultSortOrder: sortKey === 'usage' ? 'descend' : undefined,
    },
    {
      title: '成功率',
      dataIndex: 'success_rate',
      key: 'success_rate',
      render: (v: number) => (
        <Progress
          percent={Math.round(v * 100)}
          size="small"
          status={v >= 0.9 ? 'success' : v >= 0.7 ? 'normal' : 'exception'}
        />
      ),
      sorter: (a, b) => a.success_rate - b.success_rate,
      defaultSortOrder: sortKey === 'success' ? 'descend' : undefined,
    },
    {
      title: '平均耗时',
      dataIndex: 'avg_duration_ms',
      key: 'avg_duration',
      render: (v: number) => `${(v / 1000 / 60).toFixed(1)}分钟`,
    },
    {
      title: '平均成本',
      dataIndex: 'avg_cost_usd',
      key: 'avg_cost',
      render: (v: number) => `$${v.toFixed(4)}`,
      sorter: (a, b) => a.avg_cost_usd - b.avg_cost_usd,
      defaultSortOrder: sortKey === 'cost' ? 'ascend' : undefined,
    },
    {
      title: '返工率',
      dataIndex: 'rework_rate',
      key: 'rework_rate',
      render: (v: number) => (
        <Tag color={v < 0.1 ? 'green' : v < 0.3 ? 'orange' : 'red'}>
          {(v * 100).toFixed(1)}%
        </Tag>
      ),
    },
    {
      title: '用户满意度',
      dataIndex: 'user_satisfaction_score',
      key: 'satisfaction',
      render: (v: number) => (
        <Tooltip title={`${v.toFixed(1)} / 5.0`}>
          <Progress
            percent={(v / 5) * 100}
            size="small"
            strokeColor={v >= 4 ? '#52c41a' : v >= 3 ? '#faad14' : '#ff4d4f'}
          />
        </Tooltip>
      ),
    },
  ];

  return (
    <div>
      <Row justify="space-between" align="middle" style={{ marginBottom: 16 }}>
        <Col>
          <Title level={4} style={{ margin: 0 }}>管理者看板</Title>
          <Text type="secondary">
            全面了解 Agent 运营状况：效率、质量、成本、失败原因
          </Text>
        </Col>
        <Col>
          <Space>
            <Text type="secondary">时间范围:</Text>
            <RangePicker
              value={range as any}
              onChange={(dates) => setRange(dates as [Dayjs, Dayjs] | null)}
            />
          </Space>
        </Col>
      </Row>

      <Spin spinning={loading}>
        {data ? (
          <>
            <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
              <Col xs={12} sm={8} md={4}>
                <Card>
                  <Statistic
                    title="智能体总数"
                    value={data.summary.total_agents}
                    prefix={<TeamOutlined />}
                  />
                </Card>
              </Col>
              <Col xs={12} sm={8} md={4}>
                <Card>
                  <Statistic
                    title="7日对话数"
                    value={data.summary.total_conversations}
                    prefix={<MessageOutlined />}
                  />
                </Card>
              </Col>
              <Col xs={12} sm={8} md={4}>
                <Card>
                  <Statistic
                    title="7日运行数"
                    value={data.summary.total_runs_7d}
                    prefix={<ThunderboltOutlined />}
                  />
                </Card>
              </Col>
              <Col xs={12} sm={8} md={4}>
                <Card>
                  <Statistic
                    title="7日成本"
                    value={data.summary.total_cost_7d}
                    precision={2}
                    prefix={<DollarOutlined />}
                    suffix="USD"
                  />
                </Card>
              </Col>
              <Col xs={12} sm={8} md={4}>
                <Card>
                  <Statistic
                    title="平均成功率"
                    value={Math.round(data.summary.avg_success_rate * 100)}
                    prefix={<CheckCircleOutlined style={{ color: '#52c41a' }} />}
                    suffix="%"
                    valueStyle={{ color: data.summary.avg_success_rate >= 0.9 ? '#52c41a' : '#faad14' }}
                  />
                </Card>
              </Col>
              <Col xs={12} sm={8} md={4}>
                <Card>
                  <Statistic
                    title="活跃用户"
                    value={data.summary.active_users_7d}
                    prefix={<UserOutlined />}
                  />
                </Card>
              </Col>
            </Row>

            <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
              <Col xs={24} lg={12}>
                <Card title={<Space><WarningOutlined />Top 失败原因</Space>}>
                  {data.failure_reasons.length === 0 ? (
                    <Text type="secondary">暂无失败记录</Text>
                  ) : (
                    <List
                      size="small"
                      dataSource={data.failure_reasons.slice(0, 6)}
                      renderItem={(r) => (
                        <List.Item>
                          <Row style={{ width: '100%' }} align="middle">
                            <Col span={4}>
                              <Tag
                                color={r.severity === 'high' ? 'red' : r.severity === 'medium' ? 'orange' : 'blue'}
                              >
                                {r.category}
                              </Tag>
                            </Col>
                            <Col span={12}>
                              <Text>{r.description}</Text>
                            </Col>
                            <Col span={8}>
                              <Progress
                                percent={Math.round(r.percentage)}
                                size="small"
                                status={r.severity === 'high' ? 'exception' : r.severity === 'medium' ? 'active' : 'normal'}
                              />
                            </Col>
                          </Row>
                        </List.Item>
                      )}
                    />
                  )}
                </Card>
              </Col>
              <Col xs={24} lg={12}>
                <Card title={<Space><TrophyOutlined />部门效率排名</Space>}>
                  {data.departments.length === 0 ? (
                    <Text type="secondary">暂无部门数据</Text>
                  ) : (
                    <Table
                      size="small"
                      rowKey="department"
                      pagination={false}
                      columns={[
                        { title: '排名', key: 'rank', width: 60, render: (_: any, __: any, i: number) => (
                          <Text strong>#{i + 1}</Text>
                        )},
                        { title: '部门', dataIndex: 'department', key: 'dept' },
                        { title: '节省工时(h)', dataIndex: 'savings_estimate_hours', key: 'savings',
                          render: (v: number) => <Text style={{ color: '#52c41a' }}>{v.toFixed(1)}</Text> },
                        { title: 'Agent 数', dataIndex: 'agent_count', key: 'count' },
                      ]}
                      dataSource={[...data.departments].sort((a, b) => b.savings_estimate_hours - a.savings_estimate_hours)}
                    />
                  )}
                </Card>
              </Col>
            </Row>

            <Card
              title="部门效率详情"
              style={{ marginBottom: 24 }}
              extra={
                <Space>
                  <Text type="secondary">排序:</Text>
                  <Select
                    value={sortKey}
                    onChange={setSortKey}
                    style={{ width: 120 }}
                    options={[
                      { value: 'savings', label: '节省工时' },
                      { value: 'cost', label: '成本' },
                      { value: 'usage', label: '使用量' },
                    ]}
                  />
                </Space>
              }
            >
              <Table
                rowKey="department"
                columns={departmentColumns}
                dataSource={data.departments}
                pagination={{ pageSize: 10, showSizeChanger: true, showTotal: (t) => `共 ${t} 个部门` }}
              />
            </Card>

            <Card title="Agent 质量评分">
              <Table
                rowKey="agent_code"
                columns={agentColumns}
                dataSource={data.agent_metrics}
                pagination={{ pageSize: 10, showSizeChanger: true, showTotal: (t) => `共 ${t} 个 Agent` }}
                scroll={{ x: 900 }}
              />
            </Card>
          </>
        ) : (
          <Card>
            <Text type="secondary">暂无数据，请选择时间范围后重试</Text>
          </Card>
        )}
      </Spin>
    </div>
  );
}