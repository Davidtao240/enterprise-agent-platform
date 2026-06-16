import { Routes, Route, Navigate } from 'react-router-dom';
import { useAuthStore } from './store/auth';
import AppLayout from './components/AppLayout';
import LoginPage from './pages/LoginPage';
import DashboardPage from './pages/DashboardPage';
import FinanceHomePage from './pages/FinanceHomePage';
import WorkflowDetailPage from './pages/WorkflowDetailPage';
import ApprovalPage from './pages/ApprovalPage';
import AuditLogPage from './pages/AuditLogPage';

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const token = useAuthStore((s) => s.token);
  if (!token) return <Navigate to="/login" replace />;
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
        <Route path="audit-logs" element={<AuditLogPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
