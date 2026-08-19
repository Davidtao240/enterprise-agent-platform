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
import AuditLogPage from '../../AuditLogPage';
import { renderWithProviders } from '../../../test/test-utils';

const emptyStats = { total: 0, by_status: [], by_action: [] };

describe('AuditLogPage', () => {
  beforeEach(() => {
    mockApi.mockReset();
    mockApi.mockImplementation((arg: any) => {
      if (typeof arg === 'string') {
        if (arg.includes('/audit-logs/stats')) {
          return Promise.resolve({ data: { data: emptyStats } });
        }
        if (arg.includes('/audit-logs')) {
          return Promise.resolve({ data: { data: [] } });
        }
        return Promise.resolve({ data: { data: [] } });
      }
      if (typeof arg === 'object' && arg !== null) {
        if ('page_size' in arg) {
          return Promise.resolve({ data: { data: [] } });
        }
        return Promise.resolve({ data: { data: emptyStats } });
      }
      return Promise.resolve({ data: { data: [] } });
    });
  });

  it('renders audit log page title', () => {
    renderWithProviders(<AuditLogPage />);
    expect(screen.getByText('审计日志')).toBeInTheDocument();
  });

  it('renders filter form', () => {
    renderWithProviders(<AuditLogPage />);
    expect(screen.getByPlaceholderText(/追踪 ID/i)).toBeInTheDocument();
    expect(screen.getByPlaceholderText(/业务应用/i)).toBeInTheDocument();
    expect(screen.getByPlaceholderText(/动作/i)).toBeInTheDocument();
  });

  it('renders search button', () => {
    renderWithProviders(<AuditLogPage />);
    expect(screen.getByRole('button', { name: /搜.*索/i })).toBeInTheDocument();
  });

  it('renders without crashing', () => {
    const { container } = renderWithProviders(<AuditLogPage />);
    expect(container).toBeTruthy();
  });

  it('displays empty table state', async () => {
    renderWithProviders(<AuditLogPage />);
    await waitFor(() => {
      expect(screen.getByText(/暂无匹配的审计日志/i)).toBeInTheDocument();
    });
  });
});
