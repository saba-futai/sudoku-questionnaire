/**
 * Thin fetch wrapper. Everything the app sends and receives goes through here
 * so token handling and error shaping live in one place.
 */

const TOKEN_KEY = 'qrforqa:token';

export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export function getToken(): string | null {
  try {
    return localStorage.getItem(TOKEN_KEY);
  } catch {
    return null; // private mode
  }
}

export function setToken(token: string | null): void {
  try {
    if (token) localStorage.setItem(TOKEN_KEY, token);
    else localStorage.removeItem(TOKEN_KEY);
  } catch {
    /* ignore */
  }
}

/** Called when the server rejects our token, so the app can bounce to login. */
let onUnauthorized: (() => void) | null = null;
export function setUnauthorizedHandler(fn: () => void): void {
  onUnauthorized = fn;
}

interface RequestOptions {
  method?: string;
  body?: unknown;
  auth?: boolean;
  signal?: AbortSignal;
  raw?: boolean;
}

export async function api<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' };
  if (opts.body !== undefined) headers['Content-Type'] = 'application/json';

  if (opts.auth !== false) {
    const token = getToken();
    if (token) headers.Authorization = `Bearer ${token}`;
  }

  let response: Response;
  try {
    response = await fetch(path, {
      method: opts.method ?? (opts.body !== undefined ? 'POST' : 'GET'),
      headers,
      body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
      signal: opts.signal,
    });
  } catch (err) {
    if ((err as Error)?.name === 'AbortError') throw err;
    throw new ApiError(0, '网络请求失败,请检查连接后重试');
  }

  if (response.status === 401) {
    setToken(null);
    onUnauthorized?.();
  }

  if (opts.raw) {
    if (!response.ok) throw new ApiError(response.status, await readError(response));
    return (await response.blob()) as unknown as T;
  }

  const text = await response.text();
  const payload = text ? safeJson(text) : null;

  if (!response.ok) {
    throw new ApiError(response.status, extractMessage(payload, response.status));
  }
  return payload as T;
}

function safeJson(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

function extractMessage(payload: unknown, status: number): string {
  if (payload && typeof payload === 'object' && 'error' in payload) {
    const msg = (payload as { error: unknown }).error;
    if (typeof msg === 'string' && msg) return msg;
  }
  return `请求失败(${status})`;
}

async function readError(response: Response): Promise<string> {
  try {
    const text = await response.text();
    return extractMessage(safeJson(text), response.status);
  } catch {
    return `请求失败(${response.status})`;
  }
}

/**
 * Downloads an export. The endpoint is authenticated, so a plain <a href>
 * would not carry the token; the blob is fetched and saved explicitly.
 */
export async function downloadFile(path: string, fallbackName: string): Promise<void> {
  const token = getToken();
  const response = await fetch(path, {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  });
  if (!response.ok) {
    throw new ApiError(response.status, await readError(response));
  }

  const disposition = response.headers.get('Content-Disposition') ?? '';
  const name = filenameFromDisposition(disposition) ?? fallbackName;

  const blob = await response.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  // Revoke on the next tick so Safari has started the download.
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

/** Reads the RFC 5987 `filename*` parameter, falling back to `filename`. */
export function filenameFromDisposition(disposition: string): string | null {
  const star = /filename\*=UTF-8''([^;]+)/i.exec(disposition);
  if (star?.[1]) {
    try {
      return decodeURIComponent(star[1]);
    } catch {
      /* fall through */
    }
  }
  const plain = /filename="?([^";]+)"?/i.exec(disposition);
  return plain?.[1] ?? null;
}
