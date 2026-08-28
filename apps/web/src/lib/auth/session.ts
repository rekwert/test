const API_URL =
  process.env.NEXT_PUBLIC_API_URL ||
  (typeof window !== 'undefined'
    ? `${window.location.origin}/api/v1`
    : 'http://localhost:8080/api/v1');

export type AuthUser = {
  id: string;
  email: string;
  roles: string[];
  email_verified: boolean;
  locale: string;
};

export type AuthTokens = {
  access_token: string;
  refresh_token: string;
  expires_at: string;
  token_type: string;
  user: AuthUser;
};

const STORAGE_KEY = 'vps_auth';

export function saveAuth(data: AuthTokens) {
  if (typeof window !== 'undefined') {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(data));
  }
}

export function loadAuth(): AuthTokens | null {
  if (typeof window === 'undefined') return null;
  const raw = localStorage.getItem(STORAGE_KEY);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as AuthTokens;
  } catch {
    return null;
  }
}

export function clearAuth() {
  if (typeof window !== 'undefined') {
    localStorage.removeItem(STORAGE_KEY);
  }
}

export function getAccessToken(): string | null {
  return loadAuth()?.access_token ?? null;
}

function localeFromBrowser(): string {
  if (typeof navigator !== 'undefined' && navigator.language.startsWith('ru')) return 'ru';
  return 'en';
}

export async function register(email: string, password: string, locale?: string): Promise<AuthTokens> {
  const res = await fetch(`${API_URL}/auth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password, locale: locale || localeFromBrowser() }),
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || 'Registration failed');
  saveAuth(data);
  return data;
}

export async function login(email: string, password: string): Promise<AuthTokens> {
  const res = await fetch(`${API_URL}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || 'Login failed');
  saveAuth(data);
  return data;
}

export async function verifyEmail(code: string): Promise<void> {
  const token = getAccessToken();
  if (!token) throw new Error('Not authenticated');
  const res = await fetch(`${API_URL}/auth/verify-email`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
    body: JSON.stringify({ code }),
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || 'Verification failed');
  if (data.user) {
    const auth = loadAuth();
    if (auth) saveAuth({ ...auth, user: data.user });
  }
}

export async function resendVerification(): Promise<void> {
  const token = getAccessToken();
  if (!token) throw new Error('Not authenticated');
  const res = await fetch(`${API_URL}/auth/resend-verification`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || 'Resend failed');
}

export async function forgotPassword(email: string, locale?: string): Promise<void> {
  const res = await fetch(`${API_URL}/auth/forgot-password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, locale: locale || localeFromBrowser() }),
  });
  if (!res.ok) {
    const data = await res.json();
    throw new Error(data.error || 'Request failed');
  }
}

export async function resetPassword(email: string, code: string, password: string): Promise<void> {
  const res = await fetch(`${API_URL}/auth/reset-password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, code, password }),
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || 'Reset failed');
}

export async function fetchMe(): Promise<AuthUser> {
  const token = getAccessToken();
  if (!token) throw new Error('Not authenticated');
  const res = await fetch(`${API_URL}/auth/me`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || 'Unauthorized');
  return data;
}
