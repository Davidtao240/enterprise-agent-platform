import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button, Empty, Progress, Spin, Space, Tabs, Tag, Tooltip, Typography, Collapse,
} from 'antd';
import {
  CheckCircleOutlined, CloseCircleOutlined, SyncOutlined, ExclamationCircleOutlined,
  DownOutlined, UpOutlined, PlayCircleOutlined,
} from '@ant-design/icons';
import { getRunDetail } from '../../services/api';
import { buildViewModel, fmtDurationMs, ACTIVE } from './normalize';
import type { RunDetailData, RunViewModel, TodoItem } from './types';

const { Text } = Typography;

interface AgentRunPanelProps {
  runId: string | null;
  liveActive?: boolean;
  reloadTick?: number;
}

const TODO_STATUS_META: Record<TodoItem['status'], { label: string; color: string }> = {
  done: { label: '完成', color: 'success' },
  running: { label: '执行中', color: 'processing' },
  wait: { label: '待执行', color: 'default' },
  warn: { label: '待确认', color: 'warning' },
};

function parseDetail(res: unknown): RunDetailData {
  const r = (res as { data?: unknown }) ?? {};
  const layer1 = (r as { data?: unknown }).data ?? r;
  const layer2 = (layer1 as { data?: unknown }).data ?? layer1;
  return (layer2 ?? {}) as RunDetailData;
}

export default function AgentRunPanel({ runId, liveActive, reloadTick }: AgentRunPanelProps) {
  const [loading, setLoading] = useState(false);
  const [detail, setDetail] = useState<RunDetailData | null>(null);
  const [collapsed, setCollapsed] = useState(false);

  const load = useCallback(async () => {
    if (!runId) return;
    setLoading(true);
    try {
      const res = await getRunDetail(runId);
      setDetail(parseDetail(res));
    } catch {
      // 运行日志可能尚未落库或弹出一过性错误；保留旧数据。
    } finally {
      setLoading(false);
    }
  }, [runId]);

  useEffect(() => {
    setDetail(null);
    if (!runId) return;
    load();
  }, [runId, load]);

  // M1-B: reloadTick bump (SSE runtime.event) triggers an immediate refresh
  // instead of waiting for the 4s polling cadence.
  useEffect(() => {
    if (!runId || !reloadTick) return;
    load();
  }, [runId, reloadTick, load]);

  const runStatus = detail?.run?.status ?? '';
  const isActive = !!runStatus && ACTIVE.includes(runStatus);

  // running 态轮询，保持过程实时可见（SSE 步骤级事件就绪前以轮询兜底）。
  useEffect(() => {
    if (!runId || !isActive) return;
    const timer = setInterval(load, 4000);
    return () => clearInterval(timer);
  }, [runId, isActive, load]);

  const model: RunViewModel | null = useMemo(
    () => (detail ? buildViewModel(detail) : null),
    [detail],
  );

  if (!runId) return null;

  const doneCount = model?.todos.filter((t) => t.status === 'done').length ?? 0;
  const todoTotal = model?.todos.length ?? 0;

  const toolsBody = model?.toolCalls.length ? (
    <Collapse
      ghost
      items={model.toolCalls.map((t) => ({
        key: t.id,
        label: (
          <Space size={8} wrap>
            {t.status === 'ok' && <Tag color="success" icon={<CheckCircleOutlined />}>{t.name}</Tag>}
            {t.status === 'run' && <Tag color="processing" icon={<SyncOutlined spin />}>{t.name}</Tag>}
            {t.status === 'err' && <Tag color="error" icon={<CloseCircleOutlined />}>{t.name}</Tag>}
            {t.status === 'wait' && <Tag>{t.name}</Tag>}
            {t.durationMs ? <Text type="secondary" style={{ fontSize: 12 }}>{fmtDurationMs(t.durationMs)}</Text> : null}
          </Space>
        ),
        children: (
          <Space direction="vertical" style={{ width: '100%' }} size={8}>
            {t.input != null && (
              <pre style={{ margin: 0, fontSize: 11, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                <Text type="secondary">入参</Text>{'\n'}{t.input}
              </pre>
            )}
            {t.output != null && (
              <pre style={{ margin: 0, fontSize: 11, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                <Text type="secondary">出参</Text>{'\n'}{t.output}
              </pre>
            )}
            {t.error != null && (
              <pre style={{ margin: 0, fontSize: 11, whiteSpace: 'pre-wrap', color: '#cf1322' }}>
                <Text type="danger">错误</Text>{'\n'}{t.error}
              </pre>
            )}
            {t.status === 'err' && (
              <Tooltip title="后端故障重试令牌尚未接入，等待任务重入">
                <Button size="small" icon={<SyncOutlined />}>重试</Button>
              </Tooltip>
            )}
          </Space>
        ),
      }))}
    />
  ) : (
    <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无工具调用记录" />
  );

  const todosBody = model?.todos.length ? (
    <Space direction="vertical" size={8} style={{ width: '100%' }}>
      {model.todos.map((t) => {
        const meta = TODO_STATUS_META[t.status];
        return (
          <div
            key={t.id}
            style={{
              display: 'flex', alignItems: 'center', gap: 10,
              padding: '8px 10px', border: '1px solid #f0f0f0', borderRadius: 8,
              background: t.status === 'running' ? '#f0f7ff' : undefined,
            }}
          >
            {t.status === 'done' && <CheckCircleOutlined style={{ color: '#52c41a' }} />}
            {t.status === 'running' && <SyncOutlined spin style={{ color: '#1677ff' }} />}
            {t.status === 'warn' && <ExclamationCircleOutlined style={{ color: '#faad14' }} />}
            {t.status === 'wait' && <PlayCircleOutlined style={{ color: '#bfbfbf' }} />}
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ fontSize: 13, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{t.title}</div>
              <div style={{ fontSize: 11, color: 'rgba(0,0,0,0.45)' }}>{t.subtitle}</div>
            </div>
            <Tag color={meta.color}>{meta.label}</Tag>
          </div>
        );
      })}
    </Space>
  ) : (
    <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无待办步骤" />
  );

  const ctxBody = model?.contexts.length ? (
    <Space direction="vertical" size={8} style={{ width: '100%' }}>
      {model.contexts.map((c) => (
        <div
          key={c.id}
          style={{ padding: '8px 10px', border: '1px solid #f0f0f0', borderRadius: 8 }}
        >
          <div style={{ fontSize: 13 }}>{c.title}</div>
          <Text type="secondary" style={{ fontSize: 11 }}>{c.source}</Text>
          {c.score != null && (
            <Progress
              percent={Math.round(c.score * 100)}
              size="small"
              status={c.score >= 0.7 ? 'success' : c.score >= 0.5 ? 'active' : 'exception'}
              style={{ marginTop: 6 }}
            />
          )}
        </div>
      ))}
    </Space>
  ) : (
    <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无检索引用（依赖 M2 RAG 溯源）" />
  );

  const artBody = model?.artifacts.length ? (
    <Space direction="vertical" size={8} style={{ width: '100%' }}>
      {model.artifacts.map((a) => (
        <div
          key={a.id}
          style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '8px 10px', border: '1px solid #f0f0f0', borderRadius: 8 }}
        >
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ fontSize: 13, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{a.title}</div>
            <Text type="secondary" style={{ fontSize: 11 }}>{a.meta}</Text>
          </div>
          <Tag color="blue">预览</Tag>
        </div>
      ))}
    </Space>
  ) : (
    <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无产物" />
  );

  return (
    <div
      style={{
        border: '1px solid #e5e7eb', borderRadius: 12, background: '#fff',
        boxShadow: '0 4px 12px -2px rgba(17,24,39,0.08)', overflow: 'hidden',
      }}
    >
      <div
        style={{
          display: 'flex', alignItems: 'center', gap: 8, padding: '10px 14px',
          borderBottom: collapsed ? 'none' : '1px solid #f0f0f0',
        }}
      >
        <span style={{ fontWeight: 600, fontSize: 13 }}>工具面板 · 运行详情</span>
        {runStatus && <Tag color="blue">{runStatus}</Tag>}
        {liveActive && <Tag color="processing" icon={<SyncOutlined spin />}>实时</Tag>}
        <Text type="secondary" style={{ fontSize: 11, fontFamily: 'monospace' }}>
          {model?.graphKey}@{model?.graphVersion}
        </Text>
        <Button
          size="small" type="text" icon={collapsed ? <DownOutlined /> : <UpOutlined />}
          onClick={() => setCollapsed((c) => !c)}
          style={{ marginLeft: 'auto' }}
        >
          {collapsed ? '展开' : '收起'}
        </Button>
      </div>

      {!collapsed && (
        <div style={{ padding: '6px 10px 14px' }}>
          {loading && !detail ? (
            <div style={{ textAlign: 'center', padding: 24 }}><Spin /></div>
          ) : !model ? (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无运行数据" />
          ) : (
            <Tabs
              size="small"
              items={[
                {
                  key: 'todo',
                  label: `待办 ${doneCount}/${todoTotal}`,
                  children: todosBody,
                },
                {
                  key: 'tool',
                  label: `工具调用 ${model.toolCalls.length}`,
                  children: toolsBody,
                },
                {
                  key: 'ctx',
                  label: `上下文 ${model.contexts.length}`,
                  children: ctxBody,
                },
                {
                  key: 'artifact',
                  label: `产物 ${model.artifacts.length}`,
                  children: artBody,
                },
              ]}
            />
          )}
        </div>
      )}
    </div>
  );
}