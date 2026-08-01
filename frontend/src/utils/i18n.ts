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

export const workflowNodeText: Record<string, Record<string, string>> = {
  finance: {
    upload: '上传运营数据',
    agent_graph: 'AI 财务分析',
    human_review: '财务经理复核',
    archive: '归档报告',
  },
};

export const workflowNodeTypeText: Record<string, string> = {
  file_upload: '文件上传',
  agent_graph: '智能体分析',
  human_review: '人工复核',
  system: '系统处理',
};

const financeTextMap: Record<string, string> = {
  'AI analysis unavailable — showing calculated metrics only.':
    'AI 深度分析暂不可用，当前展示基于财务数据计算得到的指标。',
  'AI-generated report unavailable — showing template-based report.':
    'AI 报告生成暂不可用，当前展示基于规则模板生成的报告。',
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

export function tWorkflowNode(
  businessAppCode?: string | null,
  nodeKey?: string | null,
  fallback?: string | null,
): string {
  if (!nodeKey) return fallback || '-';
  return workflowNodeText[businessAppCode || '']?.[nodeKey] || fallback || nodeKey;
}

export function tWorkflowNodeType(nodeType?: string | null): string {
  if (!nodeType) return '-';
  return workflowNodeTypeText[nodeType] || nodeType;
}

export function localizeFinanceText(value?: string | null): string {
  if (!value) return '';
  const exact = financeTextMap[value];
  if (exact) return exact;

  const metricSummary = value.match(
    /^Total revenue:\s*([\d,.-]+)\.\s*Net profit:\s*([\d,.-]+)\s*\(margin:\s*([\d.%-]+)\)\.$/,
  );
  if (metricSummary) {
    return `营业收入合计：${metricSummary[1]}；净利润合计：${metricSummary[2]}（净利率：${metricSummary[3]}）。`;
  }

  return value;
}

export function localizeApprovalTitle(value?: string | null): string {
  if (!value) return '-';
  return value.startsWith('Review: ') ? `复核：${value.slice('Review: '.length)}` : value;
}
