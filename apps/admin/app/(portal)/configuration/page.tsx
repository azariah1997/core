import type { CSSProperties } from "react";
import { listConfig } from "../../../lib/api";
import { ApiErrorBox } from "../../../components/ApiErrorBox";

export const dynamic = "force-dynamic";

const inputStyle: CSSProperties = {
  flex: 1,
  padding: "9px 12px",
  borderRadius: 7,
  border: "1px solid var(--panel-border)",
  background: "#0e1428",
  color: "var(--text)",
  fontSize: 13.5,
};

export default async function ConfigurationPage({
  searchParams,
}: {
  searchParams: Promise<{ appId?: string; environment?: string }>;
}) {
  const { appId, environment } = await searchParams;

  let items: Awaited<ReturnType<typeof listConfig>>["items"] | null = null;
  let error: unknown = null;
  if (appId && environment) {
    try {
      items = (await listConfig(appId, environment)).items;
    } catch (e) {
      error = e;
    }
  }

  return (
    <div>
      <h1>Remote configuration</h1>
      <p className="subtitle">
        Live from GET /v1/config?appId=&amp;environment=. Both are required by the endpoint itself, since config is
        always scoped to one app and one environment - find an app&apos;s slug on the{" "}
        <a href="/applications">Applications</a> page.
      </p>
      <form method="get" className="row" style={{ marginBottom: 20, alignItems: "flex-start" }}>
        <input name="appId" defaultValue={appId || ""} placeholder="App slug or id…" style={inputStyle} />
        <input
          name="environment"
          defaultValue={environment || ""}
          placeholder="Environment (e.g. production)…"
          style={inputStyle}
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
                <th>Value</th>
                <th>Description</th>
                <th>Updated by</th>
                <th>Updated</th>
              </tr>
            </thead>
            <tbody>
              {items.length === 0 && (
                <tr>
                  <td colSpan={5} className="empty">
                    No config entries for this app/environment.
                  </td>
                </tr>
              )}
              {items.map((e) => (
                <tr key={e.id}>
                  <td className="mono">{e.key}</td>
                  <td className="mono" style={{ color: "var(--muted)" }}>
                    {JSON.stringify(e.value)}
                  </td>
                  <td>{e.description || "—"}</td>
                  <td className="mono">{e.updatedBy || "—"}</td>
                  <td>{new Date(e.updatedAt).toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
