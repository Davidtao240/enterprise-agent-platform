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
import RunDetailPage from '../../RunDetailPage';
import { renderWithProviders } from '../../../test/test-utils';

const runResponse = {
  data: {
    run: {
      id: 'run-001',
      thread_id: 'thread-001',
      trace_id: 'trace-001',
      graph_key: 'test-graph',
      graph_version: 'v1',
      status: 'succeeded',
      attempt: 1,
      output_summary_json: '{"result": "ok"}',
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      started_at: '2024-01-01T00:00:00Z',
      finished_at: '2024-01-01T00:01:00Z',
    },
    steps: [],
    events: [],
  },
};

describe('RunDetailPage', () => {
  beforeEach(() => {
    mockApi.mockReset();
    mockApi.mockImplementation((arg: any) => {
      if (typeof arg === 'string') {
        if (arg === 'run-001' || arg.includes('/runs/')) {
          return Promise.resolve({ data: runResponse });
        }
        if (arg === 'trace-001' || arg.includes('/traces/')) {
          return Promise.resolve({ data: { data: { events: [] } } });
        }
        if (arg.includes('/approval-tasks')) {
          return Promise.resolve({ data: { data: [] } });
        }
      }
      if (typeof arg === 'object' && arg !== null) {
        if (arg.status !== undefined || arg.page_size !== undefined) {
          return Promise.resolve({ data: { data: [] } });
        }
      }
      return Promise.resolve({ data: { data: {} } });
    });
  });

  it('renders run detail page title', async () => {
    renderWithProviders(<RunDetailPage />, { route: '/runs/run-001', routePattern: '/runs/:id' });
    await waitFor(() => {
      expect(screen.getByText(/Run 详情/i)).toBeInTheDocument();
    });
  });

  it('renders basic info card', async () => {
    renderWithProviders(<RunDetailPage />, { route: '/runs/run-001', routePattern: '/runs/:id' });
    await waitFor(() => {
      expect(screen.getByText('基本信息')).toBeInTheDocument();
    });
  });

  it('renders timeline section', async () => {
    renderWithProviders(<RunDetailPage />, { route: '/runs/run-001', routePattern: '/runs/:id' });
    await waitFor(() => {
      expect(screen.getByText(/时间线\s*\(\d+\s*节点\)/i)).toBeInTheDocument();
    });
  });

  it('renders without crashing', () => {
    const { container } = renderWithProviders(<RunDetailPage />, { route: '/runs/run-001', routePattern: '/runs/:id' });
    expect(container).toBeTruthy();
  });
});
