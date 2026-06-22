import axios from 'axios';

const baseURL = import.meta.env.VITE_API_BASE_URL
  ? `${import.meta.env.VITE_API_BASE_URL}/api/v1`
  : '/api/v1';

const api = axios.create({
  baseURL,
  timeout: 30000,
});

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
    if (err.response?.status === 401) {
      localStorage.removeItem('token');
      window.location.href = '/login';
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

// Workflows
export const getWorkflowTemplates = (code: string) =>
  api.get(`/business-apps/${code}/workflow-templates`);

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

export const getWorkflowNodes = (id: string) =>
  api.get(`/workflow-instances/${id}/nodes`);

// Files
export const uploadFile = (formData: FormData) =>
  api.post('/files', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  });

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

// Agent Run Logs
export const getAgentRunLogs = (params: Record<string, string>) =>
  api.get('/agent-run-logs', { params });

export default api;
