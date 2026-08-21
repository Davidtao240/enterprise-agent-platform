import { useEffect, useState, useMemo } from 'react';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import { Layout, Menu, Avatar, Dropdown, Breadcrumb, Badge, Input, theme } from 'antd';
import {
  DashboardOutlined,
  AppstoreOutlined,
  PieChartOutlined,
  ApartmentOutlined,
  BookOutlined,
  ShopOutlined,
  ApiOutlined,
  ToolOutlined,
  ExperimentOutlined,
  DatabaseOutlined,
  SafetyCertificateOutlined,
  AuditOutlined,
  UserOutlined,
  LogoutOutlined,
  SettingOutlined,
  SearchOutlined,
  BellOutlined,
  QuestionCircleOutlined,
  RobotOutlined,
} from '@ant-design/icons';
import type { MenuProps } from 'antd';
import { useAuthStore } from '../store/auth';
import { getMe } from '../services/api';

const { Header, Sider, Content } = Layout;

interface MenuItemConfig {
  key: string;
  icon?: React.ReactNode;
  label: string;
  permission?: string;
  children?: MenuItemConfig[];
}

const menuConfig: MenuItemConfig[] = [
  {
    key: '/dashboard',
    icon: <DashboardOutlined />,
    label: '工作台',
  },
  {
    key: '/gallery',
    icon: <RobotOutlined />,
    label: '智能体广场',
  },
  {
    key: 'business',
    icon: <PieChartOutlined />,
    label: '业务应用',
    children: [
      {
        key: '/finance',
        icon: <PieChartOutlined />,
        label: '财务中心',
      },
    ],
  },
  {
    key: 'management',
    icon: <SettingOutlined />,
    label: '管理中心',
    children: [
      {
        key: '/marketplace',
        icon: <ShopOutlined />,
        label: '市场',
        permission: 'agent:read',
      },
      {
        key: '/knowledge',
        icon: <BookOutlined />,
        label: '知识库',
        permission: 'tool:read',
      },
      {
        key: '/settings/connectors',
        icon: <ApartmentOutlined />,
        label: '连接器管理',
        permission: 'tool:manage',
      },
      {
        key: '/connectors-market',
        icon: <ApiOutlined />,
        label: '连接器市场',
        permission: 'tool:read',
      },
      {
        key: '/registry',
        icon: <AppstoreOutlined />,
        label: '注册中心',
        permission: 'agent:read',
      },
    ],
  },
  {
    key: 'operations',
    icon: <DatabaseOutlined />,
    label: '运维审计',
    children: [
      {
        key: '/explore/tool-calls',
        icon: <ToolOutlined />,
        label: 'Tool Call 探索器',
        permission: 'tool:read',
      },
      {
        key: '/operations/outbox',
        icon: <DatabaseOutlined />,
        label: 'Outbox 运维',
        permission: 'outbox:read',
      },
      {
        key: '/experiments',
        icon: <ExperimentOutlined />,
        label: '实验中心',
        permission: 'experiment:manage',
      },
      {
        key: '/audit-logs',
        icon: <AuditOutlined />,
        label: '审计日志',
        permission: 'audit:read',
      },
    ],
  },
  {
    key: '/rbac',
    icon: <SafetyCertificateOutlined />,
    label: '权限管理',
    permission: 'role:manage',
  },
];

const breadcrumbMap: Record<string, string> = {
  '/dashboard': '工作台',
  '/gallery': '智能体广场',
  '/finance': '财务中心',
  '/marketplace': '市场',
  '/knowledge': '知识库',
  '/settings/connectors': '连接器管理',
  '/connectors-market': '连接器市场',
  '/registry': '注册中心',
  '/explore/tool-calls': 'Tool Call 探索器',
  '/operations/outbox': 'Outbox 运维',
  '/experiments': '实验中心',
  '/audit-logs': '审计日志',
  '/rbac': '权限管理',
  '/manager-dashboard': '管理者看板',
  '/workflows': '工作流详情',
  '/approvals': '审批详情',
  '/runs': '运行详情',
  '/conversations': '对话',
};

const breadcrumbGroupMap: Record<string, string> = {
  '/finance': '业务应用',
  '/marketplace': '管理中心',
  '/knowledge': '管理中心',
  '/settings/connectors': '管理中心',
  '/connectors-market': '管理中心',
  '/registry': '管理中心',
  '/explore/tool-calls': '运维审计',
  '/operations/outbox': '运维审计',
  '/experiments': '运维审计',
  '/audit-logs': '运维审计',
  '/rbac': '系统管理',
};

function buildMenuItems(config: MenuItemConfig[], hasPermission: (p: string) => boolean): MenuProps['items'] {
  return config
    .filter((item) => !item.permission || hasPermission(item.permission))
    .map((item) => {
      if (item.children && item.children.length > 0) {
        const filteredChildren = buildMenuItems(item.children, hasPermission);
        if (!filteredChildren || filteredChildren.length === 0) return null;
        return {
          key: item.key,
          icon: item.icon,
          label: item.label,
          children: filteredChildren,
        };
      }
      return {
        key: item.key,
        icon: item.icon,
        label: item.label,
      };
    })
    .filter(Boolean) as MenuProps['items'];
}

function getBreadcrumbItems(pathname: string): { title: string }[] {
  const items: { title: string }[] = [{ title: '首页' }];

  if (pathname === '/dashboard' || pathname === '/gallery') {
    items.push({ title: pathname === '/gallery' ? '智能体广场' : '工作台' });
    return items;
  }

  const segments = pathname.split('/').filter(Boolean);
  if (segments.length === 0) return items;

  let currentPath = '';
  for (const segment of segments) {
    if (segment === 'settings') {
      currentPath += '/settings';
      const group = breadcrumbGroupMap[currentPath];
      if (group) items.push({ title: group });
      continue;
    }
    if (segment === 'explore') {
      currentPath += '/explore';
      continue;
    }
    currentPath += `/${segment}`;
    const label = breadcrumbMap[currentPath];
    if (label) {
      items.push({ title: label });
    } else if (segment === 'settings' || segment === 'explore') {
      continue;
    } else {
      const idMatch = segment.match(/^[a-f0-9-]+$/i);
      if (idMatch) {
        items.push({ title: `详情` });
      } else {
        items.push({ title: segment });
      }
    }
  }

  return items;
}

export default function AppLayout() {
  const [collapsed, setCollapsed] = useState(false);
  const navigate = useNavigate();
  const location = useLocation();
  const { token, user, permissionsLoaded, setAuth, logout, hasPermission } = useAuthStore();
  const { token: themeToken } = theme.useToken();

  const menuItems = useMemo(
    () => buildMenuItems(menuConfig, hasPermission),
    [hasPermission]
  );

  const breadcrumbItems = useMemo(
    () => getBreadcrumbItems(location.pathname),
    [location.pathname]
  );

  const userMenuItems: MenuProps['items'] = [
    {
      key: 'profile',
      icon: <UserOutlined />,
      label: '个人中心',
    },
    {
      key: 'settings',
      icon: <SettingOutlined />,
      label: '账号设置',
    },
    { type: 'divider' },
    {
      key: 'logout',
      icon: <LogoutOutlined />,
      label: '退出登录',
    },
  ];

  const handleUserMenuClick: MenuProps['onClick'] = ({ key }) => {
    if (key === 'logout') {
      logout();
      navigate('/login');
    }
  };

  const handleMenuSelect = ({ key }: { key: string }) => {
    navigate(key);
  };

  useEffect(() => {
    if (!token || permissionsLoaded) return;
    getMe().then(({ data }) => {
      setAuth(token, data.data.user, data.data.permissions || []);
    });
  }, [token, permissionsLoaded, setAuth]);

  const openKeys = useMemo(() => {
    const keys: string[] = [];
    const pathSegments = location.pathname.split('/').filter(Boolean);
    if (pathSegments.includes('finance')) keys.push('business');
    if (['marketplace', 'knowledge', 'settings', 'connectors-market', 'registry'].some(s => pathSegments.includes(s))) {
      keys.push('management');
    }
    if (['explore', 'operations', 'experiments', 'audit-logs'].some(s => pathSegments.includes(s))) {
      keys.push('operations');
    }
    return keys;
  }, [location.pathname]);

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider
        collapsible
        collapsed={collapsed}
        onCollapse={setCollapsed}
        width={220}
        style={{
          position: 'fixed',
          left: 0,
          top: 0,
          bottom: 0,
          overflow: 'auto',
          boxShadow: '2px 0 8px rgba(0, 0, 0, 0.06)',
        }}
      >
        <div
          style={{
            height: 56,
            margin: 0,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 10,
            color: '#fff',
            fontWeight: 600,
            fontSize: collapsed ? 0 : 15,
            padding: '0 16px',
            whiteSpace: 'nowrap',
            overflow: 'hidden',
            borderBottom: '1px solid rgba(255,255,255,0.08)',
          }}
        >
          <div
            style={{
              width: 32,
              height: 32,
              borderRadius: 8,
              background: 'linear-gradient(135deg, #1D4ED8 0%, #3B82F6 100%)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              flexShrink: 0,
            }}
          >
            <RobotOutlined style={{ color: '#fff', fontSize: 18 }} />
          </div>
          {!collapsed && <span>企业智能体平台</span>}
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[location.pathname]}
          defaultOpenKeys={openKeys}
          items={menuItems}
          onClick={handleMenuSelect}
          style={{ borderRight: 0, padding: '8px 0' }}
        />
      </Sider>
      <Layout style={{ marginLeft: collapsed ? 64 : 220, transition: 'margin-left 0.2s' }}>
        <Header
          style={{
            padding: '0 24px',
            background: themeToken.colorBgContainer,
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            borderBottom: '1px solid ' + themeToken.colorBorderSecondary,
            boxShadow: '0 1px 2px rgba(0, 0, 0, 0.03)',
            position: 'sticky',
            top: 0,
            zIndex: 10,
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
            <Breadcrumb items={breadcrumbItems} />
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 20 }}>
            <Input
              prefix={<SearchOutlined style={{ color: themeToken.colorTextTertiary }} />}
              placeholder="搜索..."
              style={{ width: 200, borderRadius: 8 }}
              allowClear
            />
            <Badge count={0} size="small">
              <BellOutlined style={{ fontSize: 18, color: themeToken.colorTextSecondary, cursor: 'pointer' }} />
            </Badge>
            <QuestionCircleOutlined style={{ fontSize: 18, color: themeToken.colorTextSecondary, cursor: 'pointer' }} />
            <Dropdown
              menu={{ items: userMenuItems, onClick: handleUserMenuClick }}
              placement="bottomRight"
              arrow
            >
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  cursor: 'pointer',
                  padding: '4px 8px',
                  borderRadius: 8,
                  transition: 'background 0.2s',
                }}
                onMouseEnter={(e) => (e.currentTarget.style.background = themeToken.colorBgLayout)}
                onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
              >
                <Avatar
                  style={{
                    backgroundColor: '#1D4ED8',
                    width: 32,
                    height: 32,
                  }}
                  icon={<UserOutlined />}
                />
                <span style={{ fontSize: 14, color: themeToken.colorText }}>
                  {user?.display_name || 'User'}
                </span>
              </div>
            </Dropdown>
          </div>
        </Header>
        <Content style={{ margin: 0, padding: 'var(--space-lg)', minHeight: 'calc(100vh - 56px)' }}>
          <div className="page-container" style={{ padding: 0, maxWidth: '100%' }}>
            <Outlet />
          </div>
        </Content>
      </Layout>
    </Layout>
  );
}
