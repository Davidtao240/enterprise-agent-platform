import { create } from 'zustand';

interface User {
  id: string;
  username: string;
  display_name: string;
}

function loadAuth(): { token: string | null; user: User | null; permissions: string[]; permissionsLoaded: boolean } {
  try {
    const token = localStorage.getItem('token');
    const raw = localStorage.getItem('user');
    const permissionsRaw = localStorage.getItem('permissions');
    if (!token || !raw) return { token: null, user: null, permissions: [], permissionsLoaded: false };
    return {
      token,
      user: JSON.parse(raw),
      permissions: permissionsRaw ? JSON.parse(permissionsRaw) : [],
      permissionsLoaded: permissionsRaw !== null,
    };
  } catch {
    return { token: null, user: null, permissions: [], permissionsLoaded: false };
  }
}

interface AuthState {
  token: string | null;
  user: User | null;
  permissions: string[];
  permissionsLoaded: boolean;
  setAuth: (token: string, user: User, permissions?: string[]) => void;
  hasPermission: (permission: string) => boolean;
  logout: () => void;
}

export const useAuthStore = create<AuthState>((set, get) => ({
  ...loadAuth(),
  setAuth: (token, user, permissions) => {
    const nextPermissions = permissions || [];
    localStorage.setItem('token', token);
    localStorage.setItem('user', JSON.stringify(user));
    if (permissions !== undefined) {
      localStorage.setItem('permissions', JSON.stringify(nextPermissions));
    } else {
      localStorage.removeItem('permissions');
    }
    set({ token, user, permissions: nextPermissions, permissionsLoaded: permissions !== undefined });
  },
  hasPermission: (permission) => get().permissions.includes(permission),
  logout: () => {
    localStorage.removeItem('token');
    localStorage.removeItem('user');
    localStorage.removeItem('permissions');
    set({ token: null, user: null, permissions: [], permissionsLoaded: false });
  },
}));
