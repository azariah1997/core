import { listFeatures } from "../../../lib/api";
import { ApiErrorBox } from "../../../components/ApiErrorBox";

export const dynamic = "force-dynamic";

export default async function FeaturesPage() {
  let items;
  try {
    items = (await listFeatures()).items;
  } catch (e) {
    return (
      <div>
        <h1>Feature flags</h1>
        <ApiErrorBox error={e} />
      </div>
    );
  }

  return (
    <div>
      <h1>Feature flags</h1>
      <p className="subtitle">
        Live from GET /v1/features. Read-only here - toggling flags is left to the owning application&apos;s own
        console rather than this platform-wide portal.
      </p>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Key</th>
              <th>App</th>
              <th>Enabled</th>
              <th>Description</th>
              <th>Updated</th>
            </tr>
          </thead>
          <tbody>
            {items.length === 0 && (
              <tr>
                <td colSpan={5} className="empty">
                  No feature flags.
                </td>
              </tr>
            )}
            {items.map((f) => (
              <tr key={f.id}>
                <td className="mono">{f.key}</td>
                <td className="mono" style={{ color: "var(--muted)" }}>
                  {f.appId}
                </td>
                <td>
                  <span className={`badge ${f.enabled ? "ok" : "neutral"}`}>{f.enabled ? "on" : "off"}</span>
                </td>
                <td>{f.description || "—"}</td>
                <td>{new Date(f.updatedAt).toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
