// AgentRunPanel · M1 运行过程可视化 —— 数据契约
// 由 agent_run_logs（/runs/:id）归一化为四个页签所需的视图模型。

export type StepType = 'model' | 'tool' | 'checkpoint' | 'interrupt' | 'system';
export type StepStatus = 'pending' | 'running' | 'succeeded' | 'failed' | 'cancelled';

export interface RunStep {
  id: string;
  sequence: number;
  attempt: number;
  step_type: StepType;
  name?: string;
  status: StepStatus;
  input_summary_json?: string;
  output_summary_json?: string;
  usage_json?: string;
  error_json?: string;
  started_at?: string;
  finished_at?: string;
  created_at: string;
}

export interface RuntimeEvent {
  event_id: string;
  sequence: number;
  type: string;
  payload_json?: string;
  occurred_at: string;
}

export interface RunDetailData {
  run: {
    id: string;
    graph_key: string;
    graph_version: string;
    status: string;
    attempt: number;
    output_summary_json?: string;
    error_json?: string;
    started_at?: string;
    finished_at?: string;
    created_at?: string;
  };
  steps: RunStep[];
  events: RuntimeEvent[];
}

export type TodoStatus = 'done' | 'running' | 'wait' | 'warn';

export interface TodoItem {
  id: string;
  title: string;
  subtitle: string;
  status: TodoStatus;
}

export type ToolStatus = 'ok' | 'err' | 'run' | 'wait';

export interface ToolCall {
  id: string;
  name: string;
  kind: string;
  status: ToolStatus;
  durationMs?: number;
  input?: string;
  output?: string;
  error?: string;
}

export interface ContextRef {
  id: string;
  title: string;
  source: string;
  score?: number;
}

export interface Artifact {
  id: string;
  title: string;
  meta: string;
}

export interface RunViewModel {
  runId: string;
  graphKey: string;
  graphVersion: string;
  runStatus: string;
  startedAt?: string;
  finishedAt?: string;
  todos: TodoItem[];
  toolCalls: ToolCall[];
  contexts: ContextRef[];
  artifacts: Artifact[];
}

export type RunHistoryItem = RunStep | RuntimeEvent;