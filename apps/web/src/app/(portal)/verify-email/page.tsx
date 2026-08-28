'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { resendVerification, verifyEmail } from '@/lib/auth/session';

export default function VerifyEmailPage() {
  const router = useRouter();
  const [code, setCode] = useState('');
  const [error, setError] = useState('');
  const [msg, setMsg] = useState('');
  const [loading, setLoading] = useState(false);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError('');
    setLoading(true);
    try {
      await verifyEmail(code);
      router.push('/dashboard');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Verification failed');
    } finally {
      setLoading(false);
    }
  }

  async function onResend() {
    setError('');
    setMsg('');
    try {
      await resendVerification();
      setMsg('Code sent again. Check Mailpit at localhost:8025');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Resend failed');
    }
  }

  return (
    <div style={{ maxWidth: 400 }}>
      <h1>Verify email</h1>
      <p style={{ color: '#888' }}>Enter the 6-digit code from your email.</p>
      <form onSubmit={onSubmit} style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
        <input
          type="text"
          inputMode="numeric"
          maxLength={6}
          placeholder="123456"
          value={code}
          onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))}
          required
          style={{ padding: '0.5rem', background: '#111', color: '#fff', border: '1px solid #333', borderRadius: 6, letterSpacing: '0.3em' }}
        />
        {error && <p style={{ color: '#f87171' }}>{error}</p>}
        {msg && <p style={{ color: '#4ade80' }}>{msg}</p>}
        <button type="submit" disabled={loading || code.length !== 6} style={{ padding: '0.6rem', background: '#2563eb', color: '#fff', border: 'none', borderRadius: 6 }}>
          {loading ? '...' : 'Verify'}
        </button>
      </form>
      <button type="button" onClick={onResend} style={{ marginTop: '1rem', background: 'none', border: 'none', color: '#60a5fa', cursor: 'pointer' }}>
        Resend code
      </button>
      <p style={{ marginTop: '1rem' }}><Link href="/dashboard">Skip to dashboard</Link></p>
    </div>
  );
}
