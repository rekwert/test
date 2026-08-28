import Link from 'next/link';

export default function HomePage() {
  return (
    <main style={{ padding: '2rem', fontFamily: 'system-ui' }}>
      <h1>testVPStrade</h1>
      <p>VPS hosting platform</p>
      <nav style={{ display: 'flex', gap: '1rem', marginTop: '1rem' }}>
        <Link href="/dashboard">Portal</Link>
        <Link href="/admin">Admin</Link>
      </nav>
    </main>
  );
}
