import { getSystemHealth } from "../../../lib/api";

export const dynamic = "force-dynamic";

export default async function HealthPage() {
  const results = await getSystemHealth();

  return (
    <div>
      <h1>System health</h1>
      <p className="subtitle">
        Live GET /healthz against each service&apos;s own base URL, fetched directly from this page on every load - no
        caching.
      </p>
      <div className="grid" style={{ gridTemplateColumns: "repeat(auto-fill, minmax(240px, 1fr))", gap: 14 }}>
        {results.map((r) => (
          <div className="card" key={r.name}>
            <div className="row" style={{ justifyContent: "space-between" }}>
              <strong>{r.name}</strong>
              <span className={`badge ${r.ok ? "ok" : "err"}`}>{r.ok ? "up" : "down"}</span>
            </div>
            <div className="mono" style={{ color: "var(--muted)", fontSize: 12.5, marginTop: 8 }}>
              {r.url}
            </div>
            {r.detail && (
              <div className="mono" style={{ color: "var(--muted)", fontSize: 12.5, marginTop: 4 }}>
                {r.detail}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
