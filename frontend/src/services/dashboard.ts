import api from './api';

export interface DepartmentEfficiency {
  department: string;
  agent_count: number;
  total_conversations: number;
  total_duration_ms: number;
  avg_duration_ms: number;
  cost_usd: number;
  savings_estimate_hours: number;
  top_agents: { name: string; conversations: number; savings_hours: number }[];
}

export interface AgentQualityMetric {
  agent_code: string;
  agent_name: string;
  department: string;
  total_runs: number;
  success_rate: number;
  avg_duration_ms: number;
  avg_cost_usd: number;
  human_interaction_ratio: number;
  rework_rate: number;
  user_satisfaction_score: number;
  trend_7d: { date: string; runs: number; success_rate: number }[];
}

export interface FailureReason {
  category: string;
  count: number;
  percentage: number;
  description: string;
  severity: 'low' | 'medium' | 'high';
}

export interface DashboardSummary {
  total_agents: number;
  total_conversations: number;
  total_runs_7d: number;
  total_cost_7d: number;
  avg_success_rate: number;
  active_users_7d: number;
  top_departments: { name: string; efficiency_score: number }[];
}

export interface DashboardResponse {
  summary: DashboardSummary;
  departments: DepartmentEfficiency[];
  agent_metrics: AgentQualityMetric[];
  failure_reasons: FailureReason[];
  period_days: number;
}

export async function getDashboard(params: { days?: number } = {}) {
  const res = await api.get('/dashboard', { params });
  return res.data?.data ?? res.data;
}

export async function getDepartmentEfficiency(days: number = 7) {
  const res = await api.get('/dashboard/departments', { params: { days } });
  return res.data?.data ?? res.data;
}

export async function getAgentMetrics(days: number = 7) {
  const res = await api.get('/dashboard/agents', { params: { days } });
  return res.data?.data ?? res.data;
}

export async function getFailureReasons(days: number = 7) {
  const res = await api.get('/dashboard/failures', { params: { days } });
  return res.data?.data ?? res.data;
}