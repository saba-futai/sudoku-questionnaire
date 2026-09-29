import { defineStore } from 'pinia';
import { computed, ref } from 'vue';

import { api, getToken, setToken } from '@/api/client';
import type { LoginResult } from '@/api/types';

const USERNAME_KEY = 'qrforqa:username';

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string | null>(getToken());
  const username = ref<string>(readUsername());
  const busy = ref(false);

  const isAuthenticated = computed(() => Boolean(token.value));

  async function login(user: string, password: string): Promise<void> {
    busy.value = true;
    try {
      const result = await api<LoginResult>('/api/admin/login', {
        method: 'POST',
        body: { username: user, password },
        auth: false,
      });
      token.value = result.token;
      username.value = result.username;
      setToken(result.token);
      try {
        localStorage.setItem(USERNAME_KEY, result.username);
      } catch {
        /* ignore */
      }
    } finally {
      busy.value = false;
    }
  }

  function logout(): void {
    token.value = null;
    username.value = '';
    setToken(null);
    try {
      localStorage.removeItem(USERNAME_KEY);
    } catch {
      /* ignore */
    }
  }

  async function changePassword(current: string, next: string): Promise<void> {
    await api('/api/admin/password', { method: 'POST', body: { current, next } });
  }

  return { token, username, busy, isAuthenticated, login, logout, changePassword };
});

function readUsername(): string {
  try {
    return localStorage.getItem(USERNAME_KEY) ?? '';
  } catch {
    return '';
  }
}
