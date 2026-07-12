import { useEffect, useMemo, useState } from 'react';
import { Alert, Descriptions, Drawer, Space, Table, Tabs, Tag, Typography } from 'antd';
import { getPermissionMatrix, getUserRoles } from '../services/api';
import { useAuthStore } from '../store/auth';
import { tStatus } from '../utils/i18n';

const { Title, Text } = Typography;

export default function RbacPage() {
  const hasPermission = useAuthStore((s) => s.hasPermission);
  const canReadRoles = hasPermission('role:manage');
  const canReadUsers = hasPermission('user:manage');
  const [matrix, setMatrix] = useState<any[]>([]);
  const [users, setUsers] = useState<any[]>([]);
  const [matrixLoading, setMatrixLoading] = useState(false);
  const [usersLoading, setUsersLoading] = useState(false);
  const [selectedUser, setSelectedUser] = useState<any>(null);
  const noRbacPermission = !canReadRoles && !canReadUsers;

  useEffect(() => {
    if (canReadRoles) {
      setMatrixLoading(true);
      getPermissionMatrix()
        .then(({ data }) => setMatrix(data.data || []))
        .finally(() => setMatrixLoading(false));
    }
    if (canReadUsers) {
      setUsersLoading(true);
      getUserRoles()
        .then(({ data }) => setUsers(data.data || []))
        .finally(() => setUsersLoading(false));
    }
  }, [canReadRoles, canReadUsers]);

  const roles = useMemo(
    () => Array.from(new Set(matrix.map((row) => row.role_code))).sort(),
    [matrix],
  );

  const permissionRows = useMemo(() => {
    const map = new Map<string, any>();
    matrix.forEach((row) => {
      const current = map.get(row.permission_code) || {
        permission_code: row.permission_code,
        permission_name: row.permission_name,
        resource: row.resource,
        action: row.action,
      };
      current[row.role_code] = row.granted;
      map.set(row.permission_code, current);
    });
    return Array.from(map.values()).sort((a, b) => `${a.resource}:${a.action}`.localeCompare(`${b.resource}:${b.action}`));
  }, [matrix]);

  const matrixColumns = [
    { title: '权限', dataIndex: 'permission_code', key: 'permission_code', fixed: 'left' as const, width: 220 },
    { title: '资源', dataIndex: 'resource', key: 'resource', width: 140 },
    { title: '动作', dataIndex: 'action', key: 'action', width: 140 },
    ...roles.map((role) => ({
      title: role,
      dataIndex: role,
      key: role,
      width: 150,
      render: (granted: boolean) => granted ? <Tag color="success">是</Tag> : <Tag>否</Tag>,
    })),
  ];

  const userColumns = [
    { title: '用户名', dataIndex: 'username', key: 'username' },
    { title: '显示名', dataIndex: 'display_name', key: 'display_name' },
    { title: '部门', dataIndex: 'department', key: 'department', render: (v: string) => v || '-' },
    { title: '状态', dataIndex: 'status', key: 'status', render: (status: string) => <Tag>{tStatus(status)}</Tag> },
    { title: '角色', dataIndex: 'roles', key: 'roles', render: (rolesValue: string[]) => <Space wrap>{(rolesValue || []).map((role) => <Tag key={role}>{role}</Tag>)}</Space> },
    { title: '权限数量', dataIndex: 'permissions_summary', key: 'permissions_summary', render: (items: string[]) => <Text>{(items || []).length}</Text> },
    {
      title: '',
      key: 'detail',
      render: (_: any, record: any) => <a onClick={() => setSelectedUser(record)}>详情</a>,
    },
  ];

  return (
    <div>
      <Title level={4}>权限管理</Title>
      {noRbacPermission && (
        <Alert type="info" showIcon message="当前账号没有可查看的权限管理视图" style={{ marginBottom: 16 }} />
      )}
      <Tabs
        items={[
          ...(canReadRoles ? [{
            key: 'matrix',
            label: '权限矩阵',
            children: (
              <Table
                size="small"
                dataSource={permissionRows}
                columns={matrixColumns}
                rowKey="permission_code"
                loading={matrixLoading}
                scroll={{ x: 900 + roles.length * 150 }}
                locale={{ emptyText: '暂无权限矩阵数据' }}
              />
            ),
          }] : []),
          ...(canReadUsers ? [{
            key: 'users',
            label: '用户角色',
            children: (
              <Table
                size="small"
                dataSource={users}
                columns={userColumns}
                rowKey="id"
                loading={usersLoading}
                scroll={{ x: 1100 }}
                locale={{ emptyText: '暂无用户角色数据' }}
              />
            ),
          }] : []),
        ]}
      />

      <Drawer title="用户角色详情" open={!!selectedUser} onClose={() => setSelectedUser(null)} width={680}>
        {selectedUser && (
          <Space direction="vertical" style={{ width: '100%' }} size="middle">
            <Descriptions column={1} size="small" bordered>
              <Descriptions.Item label="用户名">{selectedUser.username}</Descriptions.Item>
              <Descriptions.Item label="显示名">{selectedUser.display_name}</Descriptions.Item>
              <Descriptions.Item label="部门">{selectedUser.department || '-'}</Descriptions.Item>
              <Descriptions.Item label="状态">{tStatus(selectedUser.status)}</Descriptions.Item>
              <Descriptions.Item label="角色">
                <Space wrap>{(selectedUser.roles || []).map((role: string) => <Tag key={role}>{role}</Tag>)}</Space>
              </Descriptions.Item>
            </Descriptions>
            <Space wrap>
              {(selectedUser.permissions_summary || []).map((permission: string) => <Tag key={permission}>{permission}</Tag>)}
            </Space>
          </Space>
        )}
      </Drawer>
    </div>
  );
}
