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

import { screen, waitFor, fireEvent } from '@testing-library/react';
import AgentGalleryPage from '../index';
import { renderWithProviders } from '../../../test/test-utils';

describe('AgentGalleryPage', () => {
  beforeEach(() => {
    mockApi.mockReset();
    mockApi.mockImplementation((arg: any) => {
      if (typeof arg === 'string' && arg.includes('/agent-gallery')) {
        return Promise.resolve({ data: { data: { packages: [] } } });
      }
      return Promise.resolve({ data: { data: { packages: [] } } });
    });
  });

  it('renders page title', () => {
    renderWithProviders(<AgentGalleryPage />);
    expect(screen.getByText('智能体广场')).toBeInTheDocument();
  });

  it('renders category tabs from data', async () => {
    renderWithProviders(<AgentGalleryPage />);
    expect(screen.getByText('全部')).toBeInTheDocument();
    // SAMPLE 兜底数据中的分类会动态生成标签
    const matches = await screen.findAllByText('财务');
    expect(matches.length).toBeGreaterThan(0);
  });

  it('renders search input', () => {
    renderWithProviders(<AgentGalleryPage />);
    expect(screen.getByPlaceholderText(/搜索智能体/i)).toBeInTheDocument();
  });

  it('renders empty state when no packages match', async () => {
    renderWithProviders(<AgentGalleryPage />);
    await screen.findAllByText('财务');
    fireEvent.change(screen.getByPlaceholderText('搜索智能体...'), {
      target: { value: 'zzz-not-exist' },
    });
    await waitFor(() => {
      expect(screen.getByText('未找到匹配的智能体')).toBeInTheDocument();
    });
  });

  it('renders without crashing', () => {
    const { container } = renderWithProviders(<AgentGalleryPage />);
    expect(container).toBeTruthy();
  });
});
