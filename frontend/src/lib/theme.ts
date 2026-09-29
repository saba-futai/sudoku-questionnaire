import { ref } from 'vue';

const KEY = 'qrforqa:theme';

export type Theme = 'light' | 'dark';

/**
 * What the user actually chose. `'system'` means "no preference stored", which
 * is deliberately not the same as "the OS currently says light": the settings
 * page has to be able to show which of the three the admin picked.
 */
export type ThemePreference = Theme | 'system';

function readStored(): Theme | null {
  try {
    const stored = localStorage.getItem(KEY);
    if (stored === 'light' || stored === 'dark') return stored;
  } catch {
    /* private mode */
  }
  return null;
}

function systemTheme(): Theme {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export const themePreference = ref<ThemePreference>(readStored() ?? 'system');

export const theme = ref<Theme>(readStored() ?? systemTheme());

/**
 * Applies a theme. `persist: false` is for the "follow the system" path, where
 * writing the key would silently turn the system preference into an explicit
 * one and stop the OS from being followed from then on. Callers that pass it
 * are responsible for `themePreference` already reading correctly.
 */
export function applyTheme(next: Theme, persist = true): void {
  theme.value = next;
  document.documentElement.classList.toggle('dark', next === 'dark');
  if (!persist) return;

  themePreference.value = next;
  try {
    localStorage.setItem(KEY, next);
  } catch {
    /* ignore */
  }
}

/** Sets an explicit preference, or clears it so the OS takes over again. */
export function setThemePreference(preference: ThemePreference): void {
  if (preference === 'system') {
    try {
      localStorage.removeItem(KEY);
    } catch {
      /* ignore */
    }
    themePreference.value = 'system';
    applyTheme(systemTheme(), false);
    return;
  }
  applyTheme(preference);
}

export function toggleTheme(): void {
  applyTheme(theme.value === 'dark' ? 'light' : 'dark');
}

/** Keeps the theme in step with the OS when the user has not chosen one. */
export function watchSystemTheme(): void {
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
    if (readStored()) return;
    applyTheme(e.matches ? 'dark' : 'light', false);
  });
}
