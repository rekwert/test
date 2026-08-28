'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { resetPassword } from '@/lib/auth/session';

export default function ResetPasswordPage() {
  const router = useRouter();
  const [email, setEmail] = useState('');
  const [code, setCode] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError('');
    try {
      await resetPassword(email, code, password);
      router.push('/login');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Reset failed');
    }
  }

  return (
    <div style={{ maxWidth: 400 }}>
      <h1>Reset password</h1>
      <form onSubmit={onSubmit} style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
        <input type="email" placeholder="Email" value={email} onChange={(e) => setEmail(e.target.value)} required
          style={{ padding: '0.5rem', background: '#111', color: '#fff', border: '1px solid #333', borderRadius: 6 }} />
        <input type="text" maxLength={6} placeholder="6-digit code" value={code} onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))} required
          style={{ padding: '0.5rem', background: '#111', color: '#fff', border: '1px solid #333', borderRadius: 6 }} />
        <input type="password" placeholder="New password" value={password} onChange={(e) => setPassword(e.target.value)} required minLength={8}
          style={{ padding: '0.5rem', background: '#111', color: '#fff', border: '1px solid #333', borderRadius: 6 }} />
        {error && <p style={{ color: '#f87171' }}>{error}</p>}
        <button type="submit" style={{ padding: '0.6rem', background: '#2563eb', color: '#fff', border: 'none', borderRadius: 6 }}>Update password</button>
      </form>
      <p style={{ marginTop: '1rem' }}><Link href="/login">Back to login</Link></p>
    </div>
  );
}
