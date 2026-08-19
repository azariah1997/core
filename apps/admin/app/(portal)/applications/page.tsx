import { listApplications } from "../../../lib/api";
import { ApiErrorBox } from "../../../components/ApiErrorBox";

export const dynamic = "force-dynamic";

export default async function ApplicationsPage() {
  let items;
  try {
    items = (await listApplications()).items;
  } catch (e) {
    return (
      <div>
        <h1>Applications</h1>
        <ApiErrorBox error={e} />
      </div>
    );
  }

  return (
    <div>
      <h1>Applications</h1>
      <p className="subtitle">Live from GET /v1/apps.</p>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Slug</th>
              <th>Status</th>
              <th>Created</th>
            </tr>
          </thead>
          <tbody>
            {items.length === 0 && (
              <tr>
                <td colSpan={4} className="empty">
                  No applications.
                </td>
              </tr>
            )}
            {items.map((a) => (
              <tr key={a.id}>
                <td>{a.name}</td>
                <td className="mono">{a.slug}</td>
                <td>
                  <span className={`badge ${a.status === "active" ? "ok" : "neutral"}`}>{a.status}</span>
                </td>
                <td>{new Date(a.createdAt).toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
