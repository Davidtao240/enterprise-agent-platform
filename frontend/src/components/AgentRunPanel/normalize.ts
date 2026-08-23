// AgentRunPanel · normalize —— 把 agent_run_logs（steps + events）归一化为
// 待办 / 工具调用 / 上下文 / 产物 四类视图模型。纯函数，便于单测。

import type {
  Artifact, ContextRef, RunDetailData, RunHistoryItem, RunStep, RuntimeEvent, RunViewModel,
  StepStatus, ToolCall, TodoItem, TodoStatus, ToolStatus,
} from './types';

const ACTIVE = ['queued', 'running', 'waiting_human', 'waiting_external'];

function todoStatus(status: StepStatus): TodoStatus {
  switch (status) {
    case 'succeeded': return 'done';
    case 'running': return 'running';
    case 'failed': return 'warn';
    case 'cancelled': return 'wait';
    case 'pending':
    default: return 'wait';
  }
}

function stepLabel(step: RunStep): string {
  if (step.name) return step.name;
  const map: Record<string, string> = {
    tool: '调用工具',
    model: '模型推理',
    checkpoint: '保存检查点',
    interrupt: '等待人工确认',
    system: '系统步骤',
  };
  return map[step.step_type] ?? step.step_type;
}

// 待办：把步骤串归一化为一个可追踪的执行计划。
// interrupt 步骤标记为"待人工确认"(warn)；终末追加一个"返回结果"节点。
export function deriveTodos(steps: RunStep[], runStatus: string): TodoItem[] {
  const items: TodoItem[] = (steps || []).map((s) => {
    const interrupt = s.step_type === 'interrupt';
    return {
      id: s.id,
      title: stepLabel(s),
      subtitle: `${s.step_type} · #${s.sequence}${s.finished_at ? ` · ${fmtDurationMs(duration(s))}` : ''}`,
      status: interrupt && s.status !== 'succeeded' ? 'warn' : todoStatus(s.status),
    };
  });

  const terminal = ['succeeded', 'failed', 'cancelled'].includes(runStatus);
  if (terminal || steps.length > 0) {
    items.push({
      id: 'terminal',
      title: runStatus === 'succeeded' ? '完成并返回结果' : `流程${runStatus}`,
      subtitle: 'run 终态',
      status: runStatus === 'succeeded' ? 'done' : 'wait',
    });
  }
  return items;
}

export function duration(step: RunStep): number {
  if (!step.started_at) return 0;
  const end = step.finished_at ? new Date(step.finished_at).getTime() : 0;
  if (!end) return 0;
  return Math.max(0, end - new Date(step.started_at).getTime());
}

export function fmtDurationMs(ms: number): string {
  if (!ms) return '';
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.floor(ms / 60000)}m`;
}

function toolStatus(status: StepStatus): ToolStatus {
  switch (status) {
    case 'succeeded': return 'ok';
    case 'running': return 'run';
    case 'failed': return 'err';
    case 'cancelled':
    case 'pending':
    default: return 'wait';
  }
}

// 工具调用：合并两类来源
//   1) agent_run_steps 中 step_type=tool（V1 聚合路径）
//   2) runtime_events 中的 tool.called 事件（M1-B 细粒度实时事件）
// 若同一 tool_call_id 同时存在两条记录,以 event 为准(后者覆盖前者状态)。
export function deriveToolCalls(steps: RunStep[], events: RuntimeEvent[] = []): ToolCall[] {
  const byEventId = new Map<string, ToolCall>();
  (events || []).forEach((e) => {
    if (e.type !== 'tool.called' || !e.payload_json) return;
    try {
      const p = JSON.parse(e.payload_json);
      const id = String(p.tool_call_id ?? '');
      if (!id) return;
      const existing = byEventId.get(id);
      const next: ToolCall = {
        id,
        name: String(p.tool_name ?? id),
        kind: 'tool',
        status: (p.status ?? 'run') as ToolStatus,
        input: p.input ? String(p.input) : undefined,
        output: p.output ? String(p.output) : undefined,
        error: p.error ? (p.error.message ? String(p.error.message) : pretty(JSON.stringify(p.error))) : undefined,
      };
      if (existing && existing.status !== 'run' && next.status === 'run') {
        return; // keep terminal state
      }
      byEventId.set(id, { ...existing, ...next });
    } catch {
      /* ignore */
    }
  });

  const stepCalls = (steps || [])
    .filter((s) => s.step_type === 'tool')
    .map((s) => ({
      id: s.id,
      name: s.name || '调用工具',
      kind: 'tool',
      status: toolStatus(s.status),
      durationMs: duration(s),
      input: pretty(s.input_summary_json),
      output: pretty(s.output_summary_json),
      error: pretty(s.error_json),
    }));

  // 合并：以 event 驱动的记录为主,step 中未在 event 出现的补充进去。
  const out: ToolCall[] = [];
  byEventId.forEach((tc) => out.push(tc));
  const seen = new Set(out.map((t) => t.id));
  stepCalls.forEach((tc) => {
    if (!seen.has(tc.id)) out.push(tc);
  });
  return out;
}

// 安全地把 JSON 字符串转成可读文本；解析失败保留原文。
export function pretty(json?: string | null): string | undefined {
  if (!json) return undefined;
  try {
    const obj = JSON.parse(json);
    if (obj && typeof obj === 'object') return JSON.stringify(obj, null, 2);
    return String(obj);
  } catch {
    return json;
  }
}

// 从事件/步骤载荷中提取检索引用（拥有 score + 文档名）。
export function deriveContexts(history: RunHistoryItem[]): ContextRef[] {
  const out: ContextRef[] = [];
  (history || []).forEach((h, idx) => {
    const payload = h && 'payload_json' in h ? h.payload_json : undefined;
    const list = payload ? extractCandidates(payload) : [];
    list.forEach((c) => {
      out.push({
        id: `ctx-${idx}-${out.length}`,
        title: titleOf(c),
        source: sourceOf(c, h),
        score: c.score,
      });
    });
  });
  // 只保留有标题或来源的，避免空噪声；按分数降序，截断 6 条。
  return out
    .filter((c) => c.title || c.source)
    .sort((a, b) => (b.score ?? 0) - (a.score ?? 0))
    .slice(0, 6);
}

interface Candidate { score?: number; title?: string; name?: string; doc?: string; source?: string; collection?: string; document?: string }

function extractCandidates(payload: string): Candidate[] {
  try {
    const obj = JSON.parse(payload);
    return collect(obj).slice(0, 8);
  } catch {
    return [];
  }
}

function collect(node: unknown, acc: Candidate[] = []): Candidate[] {
  if (node && typeof node === 'object') {
    const o = node as Record<string, unknown>;
    // 判为依据：含 score 数字，且含可识别标题键。
    if (typeof o.score === 'number') {
      const title = (o.title ?? o.name ?? o.doc ?? o.document ?? o.source) as string | undefined;
      if (title) {
        acc.push({ score: o.score, title: String(title), ...o as Candidate });
        return acc;
      }
    }
    for (const v of Object.values(o)) {
      if (Array.isArray(v)) v.forEach((x) => collect(x, acc));
      else if (v && typeof v === 'object') collect(v, acc);
    }
  }
  return acc;
}

function titleOf(c: Candidate): string {
  return c.title || c.name || c.doc || c.document || c.source || '上下文引用';
}

function sourceOf(c: Candidate, h: RunHistoryItem): string {
  const coll = c.collection ? ` · ${c.collection}` : '';
  const eventType = 'type' in h ? h.type : (h.step_type || '');
  return `${eventType}${coll}${c.source && c.source !== c.title ? ` · ${c.source}` : ''}`;
}

// 产物：合并来源
//   1) run 输出 summary 中内嵌的 artifact 列表
//   2) 事件 type 或载荷中提及的文件
//   3) M1-B: runtime_events 中 artifact.ready 事件(显式声明的交付物)
export function deriveArtifacts(detail: RunDetailData): Artifact[] {
  const out: Artifact[] = [];
  const run = detail.run || {};
  if (run.output_summary_json) out.push(...extractArtifacts('run-output', run.output_summary_json));
  (detail.events || []).forEach((e, i) => {
    if (e.type === 'artifact.ready' && e.payload_json) {
      try {
        const p = JSON.parse(e.payload_json);
        out.push({
          id: `artifact-${i}-${String(p.artifact_id ?? 'x')}`,
          title: String(p.artifact_id ?? '交付物'),
          meta: Array.isArray(p.sources) ? `source: ${p.sources.join(',')}` : 'runtime event',
        });
        return;
      } catch { /* fall through */ }
    }
    const artifact = e.type || '';
    if (/artifact|report|file|result/i.test(artifact)) {
      out.push({ id: `evt-${i}`, title: artifact, meta: '事件提及' });
    } else if (e.payload_json) {
      out.push(...extractArtifacts(`evt-${i}`, e.payload_json));
    }
  });
  const seen = new Set<string>();
  const unique = out.filter((a) => {
    const k = a.title;
    if (seen.has(k)) return false;
    seen.add(k);
    return true;
  });
  if (unique.length === 0 && ['succeeded'].includes(run.status)) {
    unique.push({ id: 'output', title: '运行输出', meta: 'run.output_summary_json' });
  }
  return unique.slice(0, 6);
}

function extractArtifacts(prefix: string, json: string): Artifact[] {
  try {
    const obj = JSON.parse(json);
    return collectArtifacts(obj).slice(0, 4).map((a, i) => ({
      id: `${prefix}-${i}`,
      title: a.title,
      meta: a.meta,
    }));
  } catch {
    return [];
  }
}

function collectArtifacts(node: unknown, acc: { title: string; meta: string }[] = []): { title: string; meta: string }[] {
  if (node && typeof node === 'object') {
    const o = node as Record<string, unknown>;
    for (const k of ['artifact', 'artifacts', 'files', 'report', 'result', 'output']) {
      const v = o[k];
      if (Array.isArray(v)) {
        v.forEach((x) => {
          if (x && typeof x === 'object') {
            const xo = x as Record<string, unknown>;
            const title = (xo.title ?? xo.file_name ?? xo.name ?? xo.filename) as string | undefined;
            if (title) acc.push({ title: String(title), meta: String(xo.type ?? xo.format ?? '文件') });
          }
        });
      }
    }
    for (const v of Object.values(o)) {
      if (v && typeof v === 'object') collectArtifacts(v, acc);
      if (Array.isArray(v)) v.forEach((x) => collectArtifacts(x, acc));
    }
  }
  return acc;
}

export function buildViewModel(detail: RunDetailData): RunViewModel {
  const run = detail.run || {};
  const steps = detail.steps || [];
  const events = detail.events || [];
  const history: RunHistoryItem[] = [...steps, ...events];
  return {
    runId: run.id,
    graphKey: run.graph_key,
    graphVersion: run.graph_version,
    runStatus: run.status,
    startedAt: run.started_at,
    finishedAt: run.finished_at,
    todos: deriveTodos(steps, run.status),
    toolCalls: deriveToolCalls(steps, events),
    contexts: deriveContexts(history),
    artifacts: deriveArtifacts(detail),
  };
}

export { ACTIVE };