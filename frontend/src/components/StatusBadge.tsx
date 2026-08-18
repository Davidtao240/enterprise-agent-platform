import { Tag } from 'antd';

// 统一状态徽章(Spec WORKBENCH_DESIGN §2.2),覆盖 Run/Step/ToolCall/Outbox/实验状态。
const STATUS_COLOR: Record<string, string> = {
  // Run 状态
  queued: 'default',
  running: 'processing',
  waiting_human: 'warning',
  waiting_external: 'warning',
  succeeded: 'success',
  failed: 'error',
  cancelled: 'default',
  // Step 状态
  pending: 'default',
  // ToolCall 状态
  requested: 'default',
  pending_approval: 'warning',
  executing: 'processing',
  indeterminate: 'warning',
  // Outbox 状态
  delivered: 'success',
  compensate_pending: 'warning',
  compensated: 'success',
  dead_letter: 'error',
  // 实验/规则状态
  active: 'processing',
  stopped: 'default',
  promoted: 'success',
  rolled_back: 'error',
};

export default function StatusBadge({ status }: { status: string }) {
  return <Tag color={STATUS_COLOR[status] || 'default'}>{status}</Tag>;
}
