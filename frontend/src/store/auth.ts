import { create } from 'zustand';

interface User {
  id: string;
  username: string;
  display_name: string;
}

function loadAuth(): { token: string | null; user: User | null } {
  try {
    const token = localStorage.getItem('token');
    const raw = localStorage.getItem('user');
    if (!token || !raw) return { token: null, user: null };
    return { token, user: JSON.parse(raw) };
  } catch {
    return { token: null, user: null };
  }
}

interface AuthState {
  token: string | null;
  user: User | null;
  setAuth: (token: string, user: User) => void;
  logout: () => void;
}

export const useAuthStore = create<AuthState>((set) => ({
  ...loadAuth(),
  setAuth: (token, user) => {
    localStorage.setItem('token', token);
    localStorage.setItem('user', JSON.stringify(user));
    set({ token, user });
  },
  logout: () => {
    localStorage.removeItem('token');
    localStorage.removeItem('user');
    set({ token: null, user: null });
  },
}));
