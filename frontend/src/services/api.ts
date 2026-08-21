import axios from 'axios';
import { getErrorMessage } from '../utils/errorHandler';
import { showError } from '../utils/messageApi';

const envBase = import.meta.env.VITE_API_BASE_URL || '';
const baseURL = envBase
  ? (envBase.endsWith('/api/v1') ? envBase : `${envBase}/api/v1`)
  : '/api/v1';

const api = axios.create({
  baseURL,
  timeout: 30000,
});

let isRedirecting = false;

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

api.interceptors.response.use(
  (res) => res,
  (err) => {
    const status = err.response?.status;
    
    if (status === 401) {
      localStorage.removeItem('token');
      if (!isRedirecting) {
        isRedirecting = true;
        setTimeout(() => {
          window.location.href = '/login';
          isRedirecting = false;
        }, 100);
      }
      return Promise.reject(err);
    }

    if (status !== 401) {
      const errorData = err.response?.data?.error;
      const message = errorData?.message || getErrorMessage(err);
      if (message) {
        showError(message, 3);
      }
    }
    return Promise.reject(err);
  },
);

// Auth
export const login = (data: { username: string; password: string }) =>
  api.post('/auth/login', data);

export const getMe = () => api.get('/auth/me');

// Business Apps
export const getBusinessApps = () => api.get('/business-apps');

export const getBusinessAppRegistry = (params: Record<string, string> = {}) =>
  api.get('/business-apps/registry', { params });

export const getDomainPolicies = (params: Record<string, string> = {}) =>
  api.get('/domain-policies', { params });

// Workflows
export const getWorkflowTemplates = (code: string) =>
  api.get(`/business-apps/${code}/workflow-templates`);

export const listWorkflowTemplates = (params: Record<string, string>) =>
  api.get('/workflow-templates', { params });

export const createWorkflowInstance = (data: {
  business_app_code: string;
  workflow_template_key: string;
  title: string;
  input: Record<string, unknown>;
}) => api.post('/workflow-instances', data);

export const getWorkflowInstances = (params: Record<string, string>) =>
  api.get('/workflow-instances', { params });

export const getWorkflowInstance = (id: string) =>
  api.get(`/workflow-instances/${id}`);

export const startWorkflow = (id: string) =>
  api.post(`/workflow-instances/${id}/start`);

export const cancelWorkflow = (id: string, reason = '') =>
  api.post(`/workflow-instances/${id}/cancel`, { reason });

export const retryWorkflowNode = (id: string, nodeInstanceId: string) =>
  api.post(`/workflow-instances/${id}/retry`, { node_instance_id: nodeInstanceId });

export const getWorkflowNodes = (id: string) =>
  api.get(`/workflow-instances/${id}/nodes`);

// Files
export const uploadFile = (formData: FormData) =>
  api.post('/files', formData);

export const getFile = (id: string) =>
  api.get(`/files/${encodeURIComponent(id)}`);

export const downloadFile = (id: string) =>
  api.get(`/files/${encodeURIComponent(id)}/content`, { responseType: 'blob' });

// Approvals
export const getApprovalTasks = (params: Record<string, string>) =>
  api.get('/approval-tasks', { params });

export const getApprovalTask = (id: string) =>
  api.get(`/approval-tasks/${id}`);

export const approveTask = (id: string, comment: string) =>
  api.post(`/approval-tasks/${id}/approve`, { comment });

export const rejectTask = (id: string, comment: string) =>
  api.post(`/approval-tasks/${id}/reject`, { comment });

// Audit
export const getAuditLogs = (params: Record<string, string>) =>
  api.get('/audit-logs', { params });

export const getAuditStats = (params: Record<string, string>) =>
  api.get('/audit-logs/stats', { params });

// RBAC
export const getPermissionMatrix = () => api.get('/rbac/permission-matrix');

export const getUserRoles = () => api.get('/rbac/user-roles');

// Registry
export const getAgents = (params: Record<string, string> = {}) =>
  api.get('/agents', { params });

export const getTools = (params: Record<string, string> = {}) =>
  api.get('/tools', { params });

// Agent Run Logs
export const getAgentRunLogs = (params: Record<string, string>) =>
  api.get('/agent-run-logs', { params });

// ── M6: Workbench ──

// Runs (M6-A)
export const getRuns = (params: Record<string, string> = {}) =>
  api.get('/runs', { params });

export const getRunDetail = (id: string) =>
  api.get(`/runs/${encodeURIComponent(id)}`);

// Trace (M5-A, consumed by Run detail timeline)
export const getTrace = (traceId: string) =>
  api.get(`/traces/${encodeURIComponent(traceId)}`);

// Ops: Tool Calls & DLQ (M6-B)
export const getOpsToolCalls = (params: Record<string, string> = {}) =>
  api.get('/ops/tool-calls', { params });

export const getOpsToolCall = (id: string) =>
  api.get(`/ops/tool-calls/${encodeURIComponent(id)}`);

export const getDeadLetterToolCalls = (params: Record<string, string> = {}) =>
  api.get('/ops/tool-calls/dead-letters', { params });

// Ops: Outbox (M6-B)
export const getOutboxEntries = (params: Record<string, string> = {}) =>
  api.get('/ops/outbox', { params });

export const getOutboxEntry = (id: string) =>
  api.get(`/ops/outbox/${encodeURIComponent(id)}`);

export const compensateOutbox = (id: string, reason?: string) =>
  api.post(`/ops/outbox/${encodeURIComponent(id)}/compensate`, null, {
    params: reason ? { reason } : undefined,
  });

// Connectors (M6-C)
export const getConnectorRegistry = () => api.get('/connector-registry');

export const getConnectorBindings = () => api.get('/connector-bindings');

// Experiments (M5-C, M6-C)
export const createReplay = (data: { source_run_id: string; graph_key?: string }) =>
  api.post('/replays', data);

export const getReplay = (id: string) =>
  api.get(`/replays/${encodeURIComponent(id)}`);

export const createShadowRule = (data: {
  business_app_code: string;
  graph_key: string;
  shadow_graph_key: string;
  traffic_percent: number;
}) => api.post('/shadow-rules', data);

export const listShadowRules = () => api.get('/shadow-rules');

export const stopShadowRule = (id: string) =>
  api.post(`/shadow-rules/${encodeURIComponent(id)}/stop`);

export const listShadowExecutions = (params: Record<string, string> = {}) =>
  api.get('/shadow-executions', { params });

export const createCanaryRelease = (data: {
  business_app_code: string;
  graph_key: string;
  candidate_graph_key: string;
  stages: number[];
  max_error_rate: number;
  min_sample_size: number;
}) => api.post('/canary-releases', data);

export const listCanaryReleases = () => api.get('/canary-releases');

export const getCanaryRelease = (id: string) =>
  api.get(`/canary-releases/${encodeURIComponent(id)}`);

export const advanceCanary = (id: string) =>
  api.post(`/canary-releases/${encodeURIComponent(id)}/advance`);

export const promoteCanary = (id: string) =>
  api.post(`/canary-releases/${encodeURIComponent(id)}/promote`);

export const rollbackCanary = (id: string) =>
  api.post(`/canary-releases/${encodeURIComponent(id)}/rollback`);

export const checkCanary = (id: string) =>
  api.post(`/canary-releases/${encodeURIComponent(id)}/check`);

// Eval (M5-B, consumed by dashboard)
export const generateEvalReport = (data: {
  start_time: string;
  end_time: string;
  filters?: Record<string, string>;
  metrics?: string[];
}) => api.post('/eval/reports', data);

export default api;
