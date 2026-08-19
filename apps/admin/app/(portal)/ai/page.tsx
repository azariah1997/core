import { listAiUsage } from "../../../lib/api";
import { ApiErrorBox } from "../../../components/ApiErrorBox";

export const dynamic = "force-dynamic";

const STATUS_CLASS: Record<string, string> = {
  completed: "ok",
  failed: "err",
  pending: "neutral",
};

export default async function AiUsagePage({ searchParams }: { searchParams: Promise<{ userId?: string }> }) {
  const { userId } = await searchParams;

  let items: Awaited<ReturnType<typeof listAiUsage>>["items"] | null = null;
  let error: unknown = null;
  if (userId) {
    try {
      items = (await listAiUsage(userId)).items;
    } catch (e) {
      error = e;
    }
  }

  return (
    <div>
      <h1>AI Gateway usage</h1>
      <p className="subtitle">
        Live from GET /v1/ai/usage?userId= (Phase 24) - completions routed through the local Ollama-backed gateway,
        with token counts and cost tracked per call.
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
                <th>Model</th>
                <th>Prompt key</th>
                <th>Tokens</th>
                <th>Cost</th>
                <th>Latency</th>
                <th>Status</th>
                <th>When</th>
              </tr>
            </thead>
            <tbody>
              {items.length === 0 && (
                <tr>
                  <td colSpan={7} className="empty">
                    No completions for this user.
                  </td>
                </tr>
              )}
              {items.map((c) => (
                <tr key={c.id}>
                  <td className="mono">{c.modelAlias}</td>
                  <td className="mono" style={{ color: "var(--muted)" }}>
                    {c.promptKey || "—"}
                  </td>
                  <td>{c.totalTokens}</td>
                  <td>{c.costCents}¢</td>
                  <td>{c.latencyMs}ms</td>
                  <td>
                    <span className={`badge ${STATUS_CLASS[c.status] || "neutral"}`}>{c.status}</span>
                  </td>
                  <td>{new Date(c.createdAt).toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
