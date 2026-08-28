'use client';

import { useState } from 'react';
import Link from 'next/link';
import { forgotPassword } from '@/lib/auth/session';

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState('');
  const [msg, setMsg] = useState('');
  const [error, setError] = useState('');

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError('');
    try {
      await forgotPassword(email);
      setMsg('If the email exists, a code was sent. Check Mailpit at localhost:8025');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Request failed');
    }
  }

  return (
    <div style={{ maxWidth: 400 }}>
      <h1>Forgot password</h1>
      <form onSubmit={onSubmit} style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
        <input type="email" placeholder="Email" value={email} onChange={(e) => setEmail(e.target.value)} required
          style={{ padding: '0.5rem', background: '#111', color: '#fff', border: '1px solid #333', borderRadius: 6 }} />
        {error && <p style={{ color: '#f87171' }}>{error}</p>}
        {msg && <p style={{ color: '#4ade80' }}>{msg}</p>}
        <button type="submit" style={{ padding: '0.6rem', background: '#2563eb', color: '#fff', border: 'none', borderRadius: 6 }}>Send code</button>
      </form>
      <p style={{ marginTop: '1rem' }}><Link href="/reset-password">Already have a code?</Link></p>
    </div>
  );
}
