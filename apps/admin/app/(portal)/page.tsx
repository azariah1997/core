import Link from "next/link";
import { getSystemHealth, listApplications, listUsers } from "../../lib/api";
import { ApiErrorBox } from "../../components/ApiErrorBox";

export const dynamic = "force-dynamic";

export default async function DashboardPage() {
  const health = await getSystemHealth();

  let appCount: number | null = null;
  let userCount: number | null = null;
  let usersError: unknown = null;
  try {
    const [apps, users] = await Promise.all([listApplications(), listUsers()]);
    appCount = apps.items.length;
    userCount = users.items.length;
  } catch (e) {
    usersError = e;
  }

  return (
    <div className="stack" style={{ gap: 22 }}>
      <div>
        <h1>Dashboard</h1>
        <p className="subtitle">
          Real status pulled live from each service&apos;s own <span className="mono">/healthz</span> - this page makes no
          claim about anything it hasn&apos;t just checked.
        </p>
      </div>

      <div className="grid">
        {health.map((h) => (
          <div className="card" key={h.name}>
            <div className="row" style={{ justifyContent: "space-between" }}>
              <b>{h.name}</b>
              <span className={`badge ${h.ok ? "ok" : "err"}`}>{h.ok ? "healthy" : "unreachable"}</span>
            </div>
            <div className="mono" style={{ color: "var(--muted)", marginTop: 8, fontSize: 12 }}>
              {h.url}
            </div>
            {h.detail && (
              <div style={{ marginTop: 6, fontSize: 12.5, color: "var(--muted)" }}>{h.detail}</div>
            )}
          </div>
        ))}
      </div>

      <div className="grid">
        <div className="card">
          <div className="row" style={{ justifyContent: "space-between" }}>
            <b>Applications</b>
            <Link href="/applications">view →</Link>
          </div>
          <div style={{ fontSize: 28, fontWeight: 800, marginTop: 10 }}>{appCount ?? "—"}</div>
        </div>
        <div className="card">
          <div className="row" style={{ justifyContent: "space-between" }}>
            <b>Users (page 1)</b>
            <Link href="/users">view →</Link>
          </div>
          <div style={{ fontSize: 28, fontWeight: 800, marginTop: 10 }}>{userCount ?? "—"}</div>
        </div>
      </div>
      {usersError ? <ApiErrorBox error={usersError} /> : null}
    </div>
  );
}
