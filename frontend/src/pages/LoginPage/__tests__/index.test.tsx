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

import { screen } from '@testing-library/react';
import LoginPage from '../../LoginPage';
import { renderWithProviders } from '../../../test/test-utils';

describe('LoginPage', () => {
  beforeEach(() => {
    mockApi.mockReset();
    mockApi.mockImplementation(() => {
      return Promise.resolve({
        data: {
          data: {
            access_token: 'test-token',
            user: { id: '1', username: 'test', display_name: 'Test' },
            permissions: [],
          },
        },
      });
    });
  });

  it('renders login form with title, username, password and submit button', () => {
    renderWithProviders(<LoginPage />);
    expect(screen.getByText('企业智能体平台')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('用户名')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('密码')).toBeInTheDocument();
    expect(screen.getByRole('button')).toBeInTheDocument();
  });

  it('submit button exists and is clickable', () => {
    renderWithProviders(<LoginPage />);
    const btn = document.querySelector('button[type="submit"]');
    expect(btn).not.toBeNull();
    expect(btn?.textContent).toMatch(/登.*录/);
  });

  it('renders without crashing', () => {
    const { container } = renderWithProviders(<LoginPage />);
    expect(container).toBeTruthy();
    expect(container.querySelector('.ant-card')).toBeInTheDocument();
  });
});
