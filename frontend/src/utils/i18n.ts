export const statusText: Record<string, string> = {
  active: '启用',
  archived: '已归档',
  approved: '已通过',
  cancelled: '已取消',
  completed: '已完成',
  draft: '草稿',
  failed: '失败',
  inactive: '停用',
  pending: '待处理',
  pending_approval: '待审批',
  processing: '处理中',
  rejected: '已拒绝',
  running: '运行中',
  skipped: '已跳过',
  succeeded: '成功',
  success: '成功',
  waiting_review: '待人工审核',
};

export const riskText: Record<string, string> = {
  low: '低风险',
  medium: '中风险',
  high: '高风险',
  critical: '关键风险',
};

export const domainText: Record<string, string> = {
  finance: '财务',
  platform: '平台',
  shared: '共享',
};

export function tStatus(value?: string | null): string {
  if (!value) return '-';
  return statusText[value] || value;
}

export function tRisk(value?: string | null): string {
  if (!value) return '-';
  return riskText[value] || value;
}

export function tDomain(value?: string | null): string {
  if (!value) return '-';
  return domainText[value] || value;
}
