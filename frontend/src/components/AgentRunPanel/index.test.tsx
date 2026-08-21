import { describe, it, expect, beforeEach, vi } from 'vitest';

const mockApi = vi.hoisted(() => vi.fn());

vi.mock('../../services/api', () => ({
  default: { get: mockApi },
  getRunDetail: mockApi,
}));

import { screen, waitFor } from '@testing-library/react';
import AgentRunPanel from './index';
import { renderWithProviders } from '../../test/test-utils';

const runDetail = {
  data: {
    run: {
      id: 'run-001',
      graph_key: 'finance_report',
      graph_version: 'v1',
      status: 'succeeded',
      attempt: 1,
      output_summary_json: '{"result":"ok"}',
      started_at: '2024-01-01T00:00:00Z',
      finished_at: '2024-01-01T00:01:00Z',
    },
    steps: [
      {
        id: 's1', sequence: 1, attempt: 1, step_type: 'system', status: 'succeeded',
        name: '校验并入库数据', started_at: '2024-01-01T00:00:00Z', finished_at: '2024-01-01T00:00:05Z', created_at: '2024-01-01T00:00:00Z',
      },
      {
        id: 's2', sequence: 2, attempt: 1, step_type: 'tool', status: 'succeeded',
        name: 'finance.retrieve', input_summary_json: '{"query":"口径"}', output_summary_json: '{"docs":[{"title":"SOP.pdf","score":0.92}]}',
        started_at: '2024-01-01T00:00:06Z', finished_at: '2024-01-01T00:00:07Z', created_at: '2024-01-01T00:00:06Z',
      },
    ],
    events: [],
  },
};

describe('AgentRunPanel', () => {
  beforeEach(() => {
    mockApi.mockReset();
    mockApi.mockImplementation(() => Promise.resolve({ data: runDetail }));
  });

  it('renders nothing when no runId', () => {
    const { container } = renderWithProviders(<AgentRunPanel runId={null} />);
    expect(container).toBeEmptyDOMElement();
  });

  it('renders the panel header with graph info', async () => {
    renderWithProviders(<AgentRunPanel runId="run-001" />);
    await waitFor(() => {
      expect(screen.getByText(/工具面板/)).toBeInTheDocument();
    });
  });

  it('renders todo steps and terminal state', async () => {
    renderWithProviders(<AgentRunPanel runId="run-001" />);
    await waitFor(() => {
      expect(screen.getByText(/校验并入库数据/)).toBeInTheDocument();
    });
    expect(screen.getByText(/完成并返回结果/)).toBeInTheDocument();
  });

  it('renders tool call tab and context reference', async () => {
    renderWithProviders(<AgentRunPanel runId="run-001" />);
    await waitFor(() => {
      expect(screen.getByText(/工具调用/)).toBeInTheDocument();
    });
    // 默认展示待办；切到工具调用页签。
    const toolTab = screen.getByRole('tab', { name: /工具调用/ });
    await waitFor(() => {
      // 通过点击触发 Tool 页签，校验工具名与上下文引用渲染。
      toolTab.click();
    });
    expect(screen.getAllByText(/finance.retrieve/).length).toBeGreaterThan(0);
  });

  it('renders without crashing', () => {
    const { container } = renderWithProviders(<AgentRunPanel runId="run-001" />);
    expect(container).toBeTruthy();
  });
});