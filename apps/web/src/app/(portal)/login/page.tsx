'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { login } from '@/lib/auth/session';

export default function LoginPage() {
  const router = useRouter();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError('');
    setLoading(true);
    try {
      await login(email, password);
      router.push('/dashboard');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed');
    } finally {
      setLoading(false);
    }
  }

  return (
    <div style={{ maxWidth: 400 }}>
      <h1>Login</h1>
      <form onSubmit={onSubmit} style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
        <input
          type="email"
          placeholder="Email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          required
          style={{ padding: '0.5rem', background: '#111', color: '#fff', border: '1px solid #333', borderRadius: 6 }}
        />
        <input
          type="password"
          placeholder="Password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          required
          minLength={8}
          style={{ padding: '0.5rem', background: '#111', color: '#fff', border: '1px solid #333', borderRadius: 6 }}
        />
        {error && <p style={{ color: '#f87171' }}>{error}</p>}
        <button type="submit" disabled={loading} style={{ padding: '0.6rem', background: '#2563eb', color: '#fff', border: 'none', borderRadius: 6 }}>
          {loading ? '...' : 'Sign in'}
        </button>
      </form>
      <p style={{ marginTop: '1rem', color: '#888' }}>
        No account? <Link href="/register">Register</Link>
        {' · '}
        <Link href="/forgot-password">Forgot password?</Link>
      </p>
    </div>
  );
}
