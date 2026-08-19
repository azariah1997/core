import { listJobs } from "../../../lib/api";
import { ApiErrorBox } from "../../../components/ApiErrorBox";

export const dynamic = "force-dynamic";

const STATUS_CLASS: Record<string, string> = {
  pending: "neutral",
  running: "warn",
  completed: "ok",
  failed: "err",
};

export default async function JobsPage() {
  let items;
  try {
    items = (await listJobs()).items;
  } catch (e) {
    return (
      <div>
        <h1>Background jobs</h1>
        <ApiErrorBox error={e} />
      </div>
    );
  }

  return (
    <div>
      <h1>Background jobs</h1>
      <p className="subtitle">
        Live from GET /v1/jobs - the queue the `worker` service polls (delivery, exports, analytics rollups, and more).
      </p>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Type</th>
              <th>Status</th>
              <th>Attempts</th>
              <th>Scheduled for</th>
              <th>Updated</th>
            </tr>
          </thead>
          <tbody>
            {items.length === 0 && (
              <tr>
                <td colSpan={5} className="empty">
                  No jobs.
                </td>
              </tr>
            )}
            {items.map((j) => (
              <tr key={j.id}>
                <td className="mono">{j.type}</td>
                <td>
                  <span className={`badge ${STATUS_CLASS[j.status] || "neutral"}`}>{j.status}</span>
                </td>
                <td>
                  {j.attempts}/{j.maxAttempts}
                </td>
                <td>{j.scheduledFor ? new Date(j.scheduledFor).toLocaleString() : "—"}</td>
                <td>{new Date(j.updatedAt).toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
