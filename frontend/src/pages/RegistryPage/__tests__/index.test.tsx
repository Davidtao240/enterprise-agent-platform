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
import RegistryPage from '../../RegistryPage';
import { renderWithProviders } from '../../../test/test-utils';

describe('RegistryPage', () => {
  beforeEach(() => {
    mockApi.mockReset();
    mockApi.mockImplementation(() => {
      return Promise.resolve({ data: { data: [] } });
    });
  });

  it('renders page title', () => {
    renderWithProviders(<RegistryPage />);
    expect(screen.getByText('注册中心')).toBeInTheDocument();
  });

  it('renders tabs for different registry sections', () => {
    renderWithProviders(<RegistryPage />);
    expect(screen.getByRole('tab', { name: '业务应用' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: '智能体列表' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: '工具列表' })).toBeInTheDocument();
  });

  it('renders without crashing', () => {
    const { container } = renderWithProviders(<RegistryPage />);
    expect(container).toBeTruthy();
  });

  it('displays empty table states', async () => {
    renderWithProviders(<RegistryPage />);
    await waitFor(() => {
      expect(screen.getByText(/暂无业务应用记录/i)).toBeInTheDocument();
    });
  });
});
