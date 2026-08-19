import { listEntitlements } from "../../../lib/api";
import { ApiErrorBox } from "../../../components/ApiErrorBox";

export const dynamic = "force-dynamic";

export default async function BillingPage({ searchParams }: { searchParams: Promise<{ userId?: string }> }) {
  const { userId } = await searchParams;

  let items: Awaited<ReturnType<typeof listEntitlements>>["items"] | null = null;
  let error: unknown = null;
  if (userId) {
    try {
      items = (await listEntitlements(userId)).items;
    } catch (e) {
      error = e;
    }
  }

  return (
    <div>
      <h1>Billing / entitlements</h1>
      <p className="subtitle">
        Live from GET /v1/billing/entitlements?userId= (Phase 22). Bonus visibility, not one of the 18 roadmap-named
        areas - look up any user&apos;s entitlements by id, or find the id from their{" "}
        <a href="/users">profile page</a>, which links here automatically.
      </p>
      <form method="get" className="row" style={{ marginBottom: 20, alignItems: "flex-start" }}>
        <input
          name="userId"
          defaultValue={userId || ""}
          placeholder="User id…"
          style={{
            maxWidth: 420,
            flex: 1,
            padding: "9px 12px",
            borderRadius: 7,
            border: "1px solid var(--panel-border)",
            background: "#0e1428",
            color: "var(--text)",
            fontSize: 13.5,
          }}
        />
        <button className="btn primary small" type="submit">
          Look up
        </button>
      </form>

      {error ? <ApiErrorBox error={error} /> : null}

      {items && (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Key</th>
                <th>Source</th>
                <th>Active</th>
                <th>Granted</th>
                <th>Expires</th>
              </tr>
            </thead>
            <tbody>
              {items.length === 0 && (
                <tr>
                  <td colSpan={5} className="empty">
                    No entitlements for this user.
                  </td>
                </tr>
              )}
              {items.map((e) => (
                <tr key={e.id}>
                  <td className="mono">{e.key}</td>
                  <td className="mono" style={{ color: "var(--muted)" }}>
                    {e.source}
                  </td>
                  <td>
                    <span className={`badge ${e.active ? "ok" : "neutral"}`}>{e.active ? "active" : "inactive"}</span>
                  </td>
                  <td>{new Date(e.grantedAt).toLocaleString()}</td>
                  <td>{e.expiresAt ? new Date(e.expiresAt).toLocaleString() : "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
