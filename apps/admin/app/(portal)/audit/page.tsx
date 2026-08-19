import { listAudit } from "../../../lib/api";
import { ApiErrorBox } from "../../../components/ApiErrorBox";

export const dynamic = "force-dynamic";

export default async function AuditPage() {
  let items;
  try {
    items = (await listAudit(100)).items;
  } catch (e) {
    return (
      <div>
        <h1>Audit log</h1>
        <ApiErrorBox error={e} />
      </div>
    );
  }

  return (
    <div>
      <h1>Audit log</h1>
      <p className="subtitle">
        Live from GET /v1/audit - the append-only, trigger-enforced immutable log every write-path module records to
        (Phase 19). Most recent 100 entries.
      </p>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Occurred</th>
              <th>Action</th>
              <th>Resource</th>
              <th>Actor</th>
              <th>Correlation</th>
            </tr>
          </thead>
          <tbody>
            {items.length === 0 && (
              <tr>
                <td colSpan={5} className="empty">
                  No audit records.
                </td>
              </tr>
            )}
            {items.map((a) => (
              <tr key={a.id}>
                <td className="mono">{new Date(a.occurredAt).toLocaleString()}</td>
                <td className="mono">{a.action}</td>
                <td className="mono" style={{ color: "var(--muted)" }}>
                  {a.resourceType}/{a.resourceId}
                </td>
                <td className="mono">{a.actorUserId || "—"}</td>
                <td className="mono" style={{ color: "var(--muted)" }}>
                  {a.correlationId || "—"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
