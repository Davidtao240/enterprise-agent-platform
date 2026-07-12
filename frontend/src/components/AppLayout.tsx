import { useEffect, useState } from 'react';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import { Layout, Menu, Button, theme } from 'antd';
import {
  DashboardOutlined,
  PieChartOutlined,
  AuditOutlined,
  AppstoreOutlined,
  SafetyCertificateOutlined,
  LogoutOutlined,
} from '@ant-design/icons';
import { useAuthStore } from '../store/auth';
import { getMe } from '../services/api';

const { Header, Sider, Content } = Layout;

export default function AppLayout() {
  const [collapsed, setCollapsed] = useState(false);
  const navigate = useNavigate();
  const location = useLocation();
  const { token, user, permissionsLoaded, setAuth, logout, hasPermission } = useAuthStore();
  const { token: themeToken } = theme.useToken();
  const menuItems = [
    { key: '/', icon: <DashboardOutlined />, label: '仪表盘' },
    { key: '/finance', icon: <PieChartOutlined />, label: '财务中心' },
    ...(hasPermission('business_app:read') || hasPermission('workflow_template:read') || hasPermission('agent:manage') || hasPermission('tool:manage') ? [{ key: '/registry', icon: <AppstoreOutlined />, label: '注册中心' }] : []),
    ...(hasPermission('role:manage') || hasPermission('user:manage') ? [{ key: '/rbac', icon: <SafetyCertificateOutlined />, label: '权限管理' }] : []),
    ...(hasPermission('audit:read') ? [{ key: '/audit-logs', icon: <AuditOutlined />, label: '审计日志' }] : []),
  ];

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  useEffect(() => {
    if (!token || permissionsLoaded) return;
    getMe().then(({ data }) => {
      setAuth(token, data.data.user, data.data.permissions || []);
    });
  }, [token, permissionsLoaded, setAuth]);

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider collapsible collapsed={collapsed} onCollapse={setCollapsed}>
        <div style={{ height: 48, margin: 16, color: '#fff', textAlign: 'center', fontWeight: 600 }}>
          {collapsed ? 'EAP' : '企业智能体平台'}
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[location.pathname]}
          items={menuItems}
          onClick={({ key }) => navigate(key)}
        />
      </Sider>
      <Layout>
        <Header
          style={{
            padding: '0 24px',
            background: themeToken.colorBgContainer,
            display: 'flex',
            justifyContent: 'flex-end',
            alignItems: 'center',
            gap: 16,
          }}
        >
          <span>{user?.display_name}</span>
          <Button icon={<LogoutOutlined />} onClick={handleLogout}>
            退出登录
          </Button>
        </Header>
        <Content style={{ margin: 24 }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
}
