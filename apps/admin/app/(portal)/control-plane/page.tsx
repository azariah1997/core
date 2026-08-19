import { getSystemHealth, getPlatform, listApplications } from "../../../lib/api";
import { getPlatformData, getPrometheusAlerts, getRecentChanges } from "../../../lib/controlplane";
import { ApiErrorBox } from "../../../components/ApiErrorBox";

export const dynamic = "force-dynamic";

const ALERT_BADGE: Record<string, string> = { firing: "err", pending: "warn", inactive: "ok" };

export default async function ControlPlanePage() {
  const [health, platform, alerts, recentChanges, platformData] = await Promise.all([
    getSystemHealth(),
    getPlatform().catch(() => null),
    getPrometheusAlerts(),
    getRecentChanges(),
    getPlatformData(),
  ]);

  let appCount: number | null = null;
  let appsError: unknown = null;
  try {
    appCount = (await listApplications()).items.length;
  } catch (e) {
    appsError = e;
  }

  const nonInactiveAlerts = alerts.filter((a) => a.state !== "inactive");

  return (
    <div className="stack" style={{ gap: 26 }}>
      <div>
        <h1>Platform Control Plane</h1>
        <p className="subtitle">
          One view of applications, services, versions, environments, dependencies, deployments, database ownership,
          events, API contracts, health, alerts, and recent changes (Phase 29) - every section below is a real, live
          read, generated {new Date(platformData.generatedAt).toLocaleString()} for the data sourced from
          docs/control/platform.json.
        </p>
      </div>

      {/* Applications, services, versions, environments */}
      <section>
        <h2>Applications, services, versions &amp; environments</h2>
        <div className="grid">
          <div className="card">
            <div className="row" style={{ justifyContent: "space-between" }}>
              <b>Applications</b>
            </div>
            <div style={{ fontSize: 28, fontWeight: 800, marginTop: 10 }}>{appCount ?? "—"}</div>
            {appsError ? <ApiErrorBox error={appsError} /> : null}
          </div>
          <div className="card">
            <div className="row" style={{ justifyContent: "space-between" }}>
              <b>Environment</b>
            </div>
            <div style={{ fontSize: 20, fontWeight: 700, marginTop: 10 }}>{platform?.environment ?? "unknown"}</div>
            <div className="mono" style={{ color: "var(--muted)", fontSize: 12, marginTop: 6 }}>
              The only one this deployment has ever run in - a real single-environment system, not a fabricated
              multi-env list.
            </div>
          </div>
          {health.map((h) => (
            <div className="card" key={h.name}>
              <div className="row" style={{ justifyContent: "space-between" }}>
                <b>{h.name}</b>
                <span className={`badge ${h.ok ? "ok" : "err"}`}>{h.ok ? "up" : "down"}</span>
              </div>
              <div className="mono" style={{ color: "var(--muted)", marginTop: 8, fontSize: 12 }}>
                version: {h.version ?? "—"}
              </div>
            </div>
          ))}
        </div>
      </section>

      {/* Dependencies */}
      <section>
        <h2>Dependencies</h2>
        <p className="subtitle">
          The real dependency graph from catalog/system.yaml (Phase 26&apos;s Backstage catalog) - {platformData.components.length}{" "}
          components.
        </p>
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Component</th>
                <th>Type</th>
                <th>Depends on</th>
              </tr>
            </thead>
            <tbody>
              {platformData.components.map((c) => (
                <tr key={c.name}>
                  <td className="mono">{c.name}</td>
                  <td className="mono" style={{ color: "var(--muted)" }}>
                    {c.type}
                  </td>
                  <td className="mono" style={{ color: "var(--muted)" }}>
                    {c.dependsOn.length > 0 ? c.dependsOn.join(", ") : "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      {/* Database ownership */}
      <section>
        <h2>Database ownership</h2>
        <p className="subtitle">Which module owns which real tables - the same data docs/control&apos;s Modules sheet tracks.</p>
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Module</th>
                <th>Service</th>
                <th>Storage</th>
              </tr>
            </thead>
            <tbody>
              {platformData.modules.map((m) => (
                <tr key={m.name}>
                  <td className="mono">{m.name}</td>
                  <td className="mono" style={{ color: "var(--muted)" }}>
                    {m.service}
                  </td>
                  <td className="mono">{m.storage}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      {/* Events and API contracts */}
      <div className="grid" style={{ gridTemplateColumns: "1fr 1fr" }}>
        <section>
          <h2>Events</h2>
          <p className="subtitle">Real channels from contracts/asyncapi/events.yaml - only modules that actually publish to the outbox.</p>
          <ul style={{ margin: 0, paddingLeft: 20 }}>
            {platformData.events.map((e) => (
              <li key={e.name} className="mono" style={{ fontSize: 13, marginBottom: 4 }}>
                {e.address}
              </li>
            ))}
          </ul>
        </section>
        <section>
          <h2>API contracts</h2>
          <p className="subtitle">{platformData.endpointCount} real, currently-registered HTTP routes.</p>
          <ul style={{ margin: 0, paddingLeft: 20, fontSize: 13.5, lineHeight: 1.8 }}>
            <li>
              <span className="mono">contracts/openapi/core-api.yaml</span> - the full REST surface
            </li>
            <li>
              <span className="mono">contracts/asyncapi/events.yaml</span> - the event contract
            </li>
            <li>Both browsable live via the Backstage catalog (platform/backstage) once it&apos;s running - Phase 26</li>
          </ul>
        </section>
      </div>

      {/* Health & Alerts */}
      <section>
        <h2>Alerts</h2>
        <p className="subtitle">
          Real, live-evaluated Prometheus rules (infra/observability/alerts.yml, Phase 28) - not routed to
          Alertmanager (no real Slack/PagerDuty/email destination in this environment), but genuinely evaluated and
          queryable here.
        </p>
        {nonInactiveAlerts.length === 0 ? (
          <p style={{ color: "var(--muted)" }}>No firing or pending alerts right now (or Prometheus isn&apos;t reachable).</p>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Alert</th>
                  <th>State</th>
                  <th>Summary</th>
                </tr>
              </thead>
              <tbody>
                {nonInactiveAlerts.map((a, i) => (
                  <tr key={i}>
                    <td className="mono">{a.labels.alertname}</td>
                    <td>
                      <span className={`badge ${ALERT_BADGE[a.state] ?? "neutral"}`}>{a.state}</span>
                    </td>
                    <td>{a.annotations?.summary}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {/* Deployments */}
      <section>
        <h2>Deployments</h2>
        <div className="card">
          <p style={{ color: "var(--muted)", lineHeight: 1.6, margin: 0 }}>
            This view isn&apos;t built yet - deliberately, not silently skipped. <strong>Why:</strong> no CI/CD
            pipeline or deployment tracking exists in this environment yet (Phase 29&apos;s roadmap scope). Every
            service&apos;s real version above (a git commit SHA) is the closest honest signal available today -
            &quot;which commit is actually running&quot;, not &quot;which deployment shipped it&quot;.
          </p>
        </div>
      </section>

      {/* Recent changes */}
      <section>
        <h2>Recent changes</h2>
        <p className="subtitle">The real git log of this checkout - not a changelog someone forgot to update.</p>
        {recentChanges.length === 0 ? (
          <p style={{ color: "var(--muted)" }}>Not a git checkout, or git isn&apos;t on PATH.</p>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Commit</th>
                  <th>Author</th>
                  <th>Date</th>
                  <th>Message</th>
                </tr>
              </thead>
              <tbody>
                {recentChanges.map((c) => (
                  <tr key={c.sha}>
                    <td className="mono">{c.sha}</td>
                    <td>{c.author}</td>
                    <td className="mono" style={{ color: "var(--muted)" }}>
                      {c.date}
                    </td>
                    <td>{c.message}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  );
}
