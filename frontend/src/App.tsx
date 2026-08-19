import { Routes, Route, Navigate } from 'react-router-dom';
import { useAuthStore } from './store/auth';
import AppLayout from './components/AppLayout';
import ErrorBoundary from './components/ErrorBoundary';
import LoginPage from './pages/LoginPage';
import DashboardPage from './pages/DashboardPage';
import FinanceHomePage from './pages/FinanceHomePage';
import WorkflowDetailPage from './pages/WorkflowDetailPage';
import ApprovalPage from './pages/ApprovalPage';
import AuditLogPage from './pages/AuditLogPage';
import RegistryPage from './pages/RegistryPage';
import RbacPage from './pages/RbacPage';
import RunDetailPage from './pages/RunDetailPage';
import ToolCallExplorerPage from './pages/ToolCallExplorerPage';
import OpsOutboxPage from './pages/OpsOutboxPage';
import ExperimentsPage from './pages/ExperimentsPage';
import ConnectorScopePage from './pages/ConnectorScopePage';
import ConversationPage from './pages/ConversationPage';
import AgentGalleryPage from './pages/AgentGalleryPage';

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const token = useAuthStore((s) => s.token);
  if (!token) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

function PermissionRoute({ permission, children }: { permission: string; children: React.ReactNode }) {
  const token = useAuthStore((s) => s.token);
  const permissionsLoaded = useAuthStore((s) => s.permissionsLoaded);
  const hasPermission = useAuthStore((s) => s.hasPermission);
  if (token && !permissionsLoaded) return null;
  if (!hasPermission(permission)) return <Navigate to="/" replace />;
  return <>{children}</>;
}

function AnyPermissionRoute({ permissions, children }: { permissions: string[]; children: React.ReactNode }) {
  const token = useAuthStore((s) => s.token);
  const permissionsLoaded = useAuthStore((s) => s.permissionsLoaded);
  const hasPermission = useAuthStore((s) => s.hasPermission);
  if (token && !permissionsLoaded) return null;
  if (!permissions.some((permission) => hasPermission(permission))) return <Navigate to="/" replace />;
  return <>{children}</>;
}

export default function App() {
  return (
    <ErrorBoundary>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route
          path="/"
          element={
            <ProtectedRoute>
              <AppLayout />
            </ProtectedRoute>
          }
        >
          <Route index element={<AgentGalleryPage />} />
          <Route path="dashboard" element={<DashboardPage />} />
          <Route path="finance" element={<FinanceHomePage />} />
          <Route path="workflows/:id" element={<WorkflowDetailPage />} />
          <Route path="approvals/:id" element={<ApprovalPage />} />
          <Route path="registry" element={<AnyPermissionRoute permissions={['business_app:read', 'workflow_template:read', 'agent:manage', 'tool:manage']}><RegistryPage /></AnyPermissionRoute>} />
          <Route path="rbac" element={<AnyPermissionRoute permissions={['role:manage', 'user:manage']}><RbacPage /></AnyPermissionRoute>} />
          <Route path="audit-logs" element={<PermissionRoute permission="audit:read"><AuditLogPage /></PermissionRoute>} />
          {/* M6: Enterprise Workbench(Spec WORKBENCH_DESIGN §6.4) */}
          <Route path="runs/:id" element={<PermissionRoute permission="workflow:read"><RunDetailPage /></PermissionRoute>} />
          <Route path="explore/tool-calls" element={<PermissionRoute permission="tool:read"><ToolCallExplorerPage /></PermissionRoute>} />
          <Route path="operations/outbox" element={<PermissionRoute permission="outbox:read"><OpsOutboxPage /></PermissionRoute>} />
          <Route path="experiments" element={<PermissionRoute permission="experiment:manage"><ExperimentsPage /></PermissionRoute>} />
          <Route path="settings/connectors" element={<PermissionRoute permission="tool:manage"><ConnectorScopePage /></PermissionRoute>} />
          <Route path="conversations/:id" element={<ConversationPage />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </ErrorBoundary>
  );
}
