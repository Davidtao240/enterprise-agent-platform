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
import ExperimentsPage from '../../ExperimentsPage';
import { renderWithProviders } from '../../../test/test-utils';

describe('ExperimentsPage', () => {
  beforeEach(() => {
    mockApi.mockReset();
    mockApi.mockImplementation((arg: any) => {
      if (typeof arg === 'string') {
        if (arg.includes('/canary-releases')) {
          return Promise.resolve({ data: { data: { items: [] } } });
        }
        if (arg.includes('/shadow-rules')) {
          return Promise.resolve({ data: { data: { items: [] } } });
        }
        if (arg.includes('/shadow-executions')) {
          return Promise.resolve({ data: { data: { items: [] } } });
        }
        if (arg.includes('/replays')) {
          return Promise.resolve({ data: { data: {} } });
        }
      }
      if (typeof arg === 'undefined' || arg === null) {
        return Promise.resolve({ data: { data: { items: [] } } });
      }
      if (typeof arg === 'object' && arg !== null) {
        if ('source_run_id' in arg) {
          return Promise.resolve({ data: { data: { id: 'replay-001' } } });
        }
        if ('business_app_code' in arg) {
          return Promise.resolve({ data: { data: {} } });
        }
        return Promise.resolve({ data: { data: { items: [] } } });
      }
      return Promise.resolve({ data: { data: { items: [] } } });
    });
  });

  it('renders experiments page title', () => {
    renderWithProviders(<ExperimentsPage />);
    expect(screen.getByText(/实验中心/i)).toBeInTheDocument();
  });

  it('renders canary tab', () => {
    renderWithProviders(<ExperimentsPage />);
    expect(screen.getByText('Canary 发布')).toBeInTheDocument();
  });

  it('renders shadow tab', () => {
    renderWithProviders(<ExperimentsPage />);
    expect(screen.getByText('Shadow 流量')).toBeInTheDocument();
  });

  it('renders replay tab', () => {
    renderWithProviders(<ExperimentsPage />);
    expect(screen.getByText('Replay 重放')).toBeInTheDocument();
  });

  it('renders without crashing', () => {
    const { container } = renderWithProviders(<ExperimentsPage />);
    expect(container).toBeTruthy();
  });

  it('renders create buttons for canary and shadow', async () => {
    renderWithProviders(<ExperimentsPage />);
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /创建发布/i })).toBeInTheDocument();
    });
  });
});
