export default function ServerDetailPage({ params }: { params: { id: string } }) {
  return (
    <>
      <h1>Server #{params.id}</h1>
      <p>Instance card with power actions: reboot, shutdown, VNC (phase 2).</p>
      <div style={{ display: 'flex', gap: '0.5rem', marginTop: '1rem' }}>
        {['Restart', 'Shutdown', 'VNC console', 'Reinstall OS', 'Delete'].map((action) => (
          <button key={action} style={{ padding: '0.5rem 1rem', background: '#222', color: '#fff', border: '1px solid #444', borderRadius: 6 }}>
            {action}
          </button>
        ))}
      </div>
    </>
  );
}
