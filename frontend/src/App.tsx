import { Routes, Route, Navigate } from 'react-router-dom';
import { useAuthStore } from './store/auth';
import AppLayout from './components/AppLayout';
import LoginPage from './pages/LoginPage';
import DashboardPage from './pages/DashboardPage';
import FinanceHomePage from './pages/FinanceHomePage';
import WorkflowDetailPage from './pages/WorkflowDetailPage';
import ApprovalPage from './pages/ApprovalPage';
import AuditLogPage from './pages/AuditLogPage';
import RegistryPage from './pages/RegistryPage';

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
        <Route index element={<DashboardPage />} />
        <Route path="finance" element={<FinanceHomePage />} />
        <Route path="workflows/:id" element={<WorkflowDetailPage />} />
        <Route path="approvals/:id" element={<ApprovalPage />} />
        <Route path="registry" element={<AnyPermissionRoute permissions={['agent:manage', 'tool:manage']}><RegistryPage /></AnyPermissionRoute>} />
        <Route path="audit-logs" element={<PermissionRoute permission="audit:read"><AuditLogPage /></PermissionRoute>} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
