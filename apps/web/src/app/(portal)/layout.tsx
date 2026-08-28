'use client';

import Link from 'next/link';
import { useEffect, useState } from 'react';
import { clearAuth, loadAuth, type AuthTokens } from '@/lib/auth/session';

export default function PortalLayout({ children }: { children: React.ReactNode }) {
  const [auth, setAuth] = useState<AuthTokens | null>(null);

  useEffect(() => {
    setAuth(loadAuth());
  }, []);

  return (
    <div style={{ display: 'flex', minHeight: '100vh' }}>
      <aside style={{ width: 240, padding: '1rem', borderRight: '1px solid #333', background: '#111' }}>
        <h2 style={{ color: '#fff' }}>Portal</h2>
        {auth?.user && (
          <p style={{ color: '#666', fontSize: 12, marginBottom: '1rem' }}>{auth.user.email}</p>
        )}
        <nav style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
          <Link href="/dashboard" style={{ color: '#aaa' }}>My servers</Link>
          <Link href="/login" style={{ color: '#aaa' }}>Login</Link>
          <Link href="/register" style={{ color: '#aaa' }}>Register</Link>
          <button
            type="button"
            onClick={() => { clearAuth(); setAuth(null); window.location.href = '/login'; }}
            style={{ textAlign: 'left', background: 'none', border: 'none', color: '#aaa', cursor: 'pointer', padding: 0 }}
          >
            Logout
          </button>
        </nav>
      </aside>
      <main style={{ flex: 1, padding: '1.5rem', background: '#0a0a0a', color: '#eee' }}>
        {children}
      </main>
    </div>
  );
}
