import React from 'react';
import { render, RenderOptions } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { ConfigProvider } from 'antd';
import { useAuthStore } from '../store/auth';

interface TestProviderProps {
  children: React.ReactNode;
  route?: string;
  routePattern?: string;
  token?: string;
  permissions?: string[];
}

function TestProvider({
  children,
  route = '/',
  routePattern,
  token = 'test-token',
  permissions = [
    'workflow:read',
    'agent:read',
    'tool:read',
    'agent:manage',
    'tool:manage',
    'experiment:manage',
    'audit:read',
    'business_app:read',
    'workflow_template:read',
    'role:manage',
    'user:manage',
    'approval:decide',
    'approval:read',
    'trace:read',
  ],
}: TestProviderProps) {
  useAuthStore.setState({
    token,
    user: { id: '1', username: 'test', display_name: 'Test User' },
    permissions,
    permissionsLoaded: true,
  });
  const content = routePattern ? (
    <Routes>
      <Route path={routePattern} element={children} />
    </Routes>
  ) : children;
  return (
    <ConfigProvider>
      <MemoryRouter initialEntries={[route]}>{content}</MemoryRouter>
    </ConfigProvider>
  );
}

export function renderWithProviders(
  ui: React.ReactElement,
  options?: Omit<RenderOptions, 'wrapper'> & {
    route?: string;
    routePattern?: string;
    token?: string;
    permissions?: string[];
  }
) {
  const { route, routePattern, token, permissions, ...renderOptions } = options || {};
  return render(ui, {
    wrapper: (props) => (
      <TestProvider route={route} routePattern={routePattern} token={token} permissions={permissions}>
        {props.children}
      </TestProvider>
    ),
    ...renderOptions,
  });
}