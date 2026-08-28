export default function AdminLayout({ children }: { children: React.ReactNode }) {
  return (
    <div style={{ display: 'flex', minHeight: '100vh' }}>
      <aside style={{ width: 240, padding: '1rem', borderRight: '1px solid #333', background: '#0f172a' }}>
        <h2 style={{ color: '#fff' }}>Admin</h2>
        <nav style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
          <a href="/admin/users" style={{ color: '#94a3b8' }}>Users</a>
          <a href="/admin/instances" style={{ color: '#94a3b8' }}>Instances</a>
          <a href="/admin/plans" style={{ color: '#94a3b8' }}>Plans</a>
          <a href="/admin/audit" style={{ color: '#94a3b8' }}>Audit log</a>
        </nav>
      </aside>
      <main style={{ flex: 1, padding: '1.5rem' }}>{children}</main>
    </div>
  );
}
