import { describe, it, expect, beforeEach, vi } from 'vitest';

const mockApi = vi.hoisted(() => vi.fn());
const mockSSE = vi.hoisted(() => vi.fn());

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

vi.mock('../../../services/sse', () => ({
  createSSEConnection: mockSSE,
}));

import { screen, waitFor } from '@testing-library/react';
import ConversationPage from '../index';
import { renderWithProviders } from '../../../test/test-utils';

describe('ConversationPage', () => {
  beforeEach(() => {
    mockApi.mockReset();
    mockSSE.mockReset();
    mockApi.mockImplementation((arg: any) => {
      if (typeof arg === 'string' && arg.startsWith('/conversations/')) {
        return Promise.resolve({
          data: {
            data: {
              id: 'test-conv-id',
              agent_package_code: 'test-agent',
              title: 'Test Conversation',
              status: 'active',
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z',
              messages: [],
            },
          },
        });
      }
      return Promise.resolve({ data: { data: [] } });
    });
    mockSSE.mockImplementation(() => ({
      close: vi.fn(),
      on: vi.fn(() => vi.fn()),
    }));
  });

  it('renders without crashing', () => {
    const { container } = renderWithProviders(<ConversationPage />, { route: '/conversations/test-id', routePattern: '/conversations/:id' });
    expect(container).toBeTruthy();
  });

  it('eventually renders chat area', async () => {
    renderWithProviders(<ConversationPage />, { route: '/conversations/test-id', routePattern: '/conversations/:id' });
    await waitFor(() => {
      expect(screen.getByText(/新建对话|暂无对话/i)).toBeInTheDocument();
    });
  });
});
