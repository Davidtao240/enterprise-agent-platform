import { describe, it, expect, beforeEach, vi } from 'vitest';

const mockApi = vi.hoisted(() => vi.fn());

vi.mock('../../../services/api', () => ({
  default: { get: mockApi, post: mockApi, put: mockApi, patch: mockApi, delete: mockApi },
  login: mockApi,
  getMe: mockApi,
  getBusinessApps: mockApi,
  getBusinessAppRegistry: mockApi,
  getDomainPolicies: mockApi,
  getWorkflowTemplates: mockApi,
  listWorkflowTemplates: mockApi,
  createWorkflowInstance: mockApi,
  getWorkflowInstances: mockApi,
  getWorkflowInstance: mockApi,
  startWorkflow: mockApi,
  cancelWorkflow: mockApi,
  retryWorkflowNode: mockApi,
  getWorkflowNodes: mockApi,
  uploadFile: mockApi,
  getFile: mockApi,
  downloadFile: mockApi,
  getApprovalTasks: mockApi,
  getApprovalTask: mockApi,
  approveTask: mockApi,
  rejectTask: mockApi,
  getAuditLogs: mockApi,
  getAuditStats: mockApi,
  getPermissionMatrix: mockApi,
  getUserRoles: mockApi,
  getAgents: mockApi,
  getTools: mockApi,
  getAgentRunLogs: mockApi,
  getRuns: mockApi,
  getRunDetail: mockApi,
  getTrace: mockApi,
  getOpsToolCalls: mockApi,
  getOpsToolCall: mockApi,
  getDeadLetterToolCalls: mockApi,
  getOutboxEntries: mockApi,
  getOutboxEntry: mockApi,
  compensateOutbox: mockApi,
  getConnectorRegistry: mockApi,
  getConnectorBindings: mockApi,
  createReplay: mockApi,
  getReplay: mockApi,
  createShadowRule: mockApi,
  listShadowRules: mockApi,
  stopShadowRule: mockApi,
  listShadowExecutions: mockApi,
  createCanaryRelease: mockApi,
  listCanaryReleases: mockApi,
  getCanaryRelease: mockApi,
  advanceCanary: mockApi,
  promoteCanary: mockApi,
  rollbackCanary: mockApi,
  checkCanary: mockApi,
  generateEvalReport: mockApi,
}));

import { screen, waitFor } from '@testing-library/react';
import ApprovalPage from '../../ApprovalPage';
import { renderWithProviders } from '../../../test/test-utils';

const mockApprovalTask = {
  id: 'task-001',
  title: 'Review: 月度财务报表',
  workflow_title: '财务报表审批流程',
  status: 'pending',
  workflow_instance_id: 'wf-001',
  agent_output_json: '{"summary": "营业收入合计：1,234,567。"}',
};

const mockWorkflowInstance = {
  input_json: '{"file_id": "file-001"}',
};

const mockFile = {
  id: 'file-001',
  original_filename: 'report.xlsx',
  size_bytes: 102400,
  content_type: 'application/xlsx',
};

describe('ApprovalPage', () => {
  beforeEach(() => {
    mockApi.mockReset();
    mockApi.mockImplementation((arg: any) => {
      if (typeof arg === 'string') {
        if (arg === 'task-001') {
          return Promise.resolve({ data: { data: mockApprovalTask } });
        }
        if (arg === 'wf-001') {
          return Promise.resolve({ data: { data: mockWorkflowInstance } });
        }
        if (arg === 'file-001') {
          return Promise.resolve({ data: { data: mockFile } });
        }
        if (arg.includes('/workflow-instances/')) {
          return Promise.resolve({ data: { data: mockWorkflowInstance } });
        }
        if (arg.includes('/files/')) {
          return Promise.resolve({ data: { data: mockFile } });
        }
      }
      return Promise.resolve({ data: { data: {} } });
    });
  });

  it('renders approval page title', async () => {
    renderWithProviders(<ApprovalPage />, { route: '/approvals/task-001', routePattern: '/approvals/:id' });
    await waitFor(() => {
      expect(screen.getByText('审批复核')).toBeInTheDocument();
    });
  });

  it('renders approve and reject buttons', async () => {
    renderWithProviders(<ApprovalPage />, { route: '/approvals/task-001', routePattern: '/approvals/:id' });
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /通.*过/i })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /拒.*绝/i })).toBeInTheDocument();
    });
  });

  it('renders comment textarea', async () => {
    renderWithProviders(<ApprovalPage />, { route: '/approvals/task-001', routePattern: '/approvals/:id' });
    await waitFor(() => {
      expect(screen.getByPlaceholderText(/请输入审批意见/i)).toBeInTheDocument();
    });
  });

  it('renders without crashing', () => {
    const { container } = renderWithProviders(<ApprovalPage />, { route: '/approvals/task-001', routePattern: '/approvals/:id' });
    expect(container).toBeTruthy();
  });
});
