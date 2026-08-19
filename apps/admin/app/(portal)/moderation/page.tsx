import { listModerationCases } from "../../../lib/api";
import { ApiErrorBox } from "../../../components/ApiErrorBox";

export const dynamic = "force-dynamic";

const STATUS_CLASS: Record<string, string> = {
  open: "warn",
  resolved: "ok",
  dismissed: "neutral",
};

export default async function ModerationPage() {
  let items;
  try {
    items = (await listModerationCases()).items;
  } catch (e) {
    return (
      <div>
        <h1>Moderation cases</h1>
        <ApiErrorBox error={e} />
      </div>
    );
  }

  return (
    <div>
      <h1>Moderation cases</h1>
      <p className="subtitle">
        Live from GET /v1/trustsafety/cases (Phase 21) - user reports deduplicated into cases, gated to moderators via
        authz&apos;s IsModerator check at the HTTP layer.
      </p>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Resource</th>
              <th>Status</th>
              <th>Reports</th>
              <th>Assigned to</th>
              <th>Resolution</th>
              <th>Created</th>
            </tr>
          </thead>
          <tbody>
            {items.length === 0 && (
              <tr>
                <td colSpan={6} className="empty">
                  No cases.
                </td>
              </tr>
            )}
            {items.map((c) => (
              <tr key={c.id}>
                <td className="mono" style={{ color: "var(--muted)" }}>
                  {c.resourceType}/{c.resourceId}
                </td>
                <td>
                  <span className={`badge ${STATUS_CLASS[c.status] || "neutral"}`}>{c.status}</span>
                </td>
                <td>{c.reportCount}</td>
                <td className="mono">{c.assignedTo || "—"}</td>
                <td>{c.resolution || "—"}</td>
                <td>{new Date(c.createdAt).toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
