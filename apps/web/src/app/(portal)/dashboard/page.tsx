export default function DashboardPage() {
  return (
    <>
      <h1>My servers</h1>
      <p>VPS management, tariffs, and renewals.</p>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '1rem', margin: '1.5rem 0' }}>
        {['Active VPS', 'Needs renewal', 'Total VPS', 'With backups'].map((label) => (
          <div key={label} style={{ padding: '1rem', background: '#1a1a1a', borderRadius: 8 }}>
            <div style={{ color: '#888', fontSize: 14 }}>{label}</div>
            <div style={{ fontSize: 24, fontWeight: 700 }}>0</div>
          </div>
        ))}
      </div>
      <p style={{ color: '#666' }}>Server list will connect to /api/v1/instances</p>
    </>
  );
}
