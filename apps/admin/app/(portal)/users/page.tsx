import Link from "next/link";
import { listUsers } from "../../../lib/api";
import { ApiErrorBox } from "../../../components/ApiErrorBox";

export const dynamic = "force-dynamic";

export default async function UsersPage({ searchParams }: { searchParams: Promise<{ cursor?: string }> }) {
  const { cursor } = await searchParams;
  let result;
  try {
    result = await listUsers(cursor);
  } catch (e) {
    return (
      <div>
        <h1>Users</h1>
        <ApiErrorBox error={e} />
      </div>
    );
  }

  return (
    <div>
      <h1>Users</h1>
      <p className="subtitle">
        Live from GET /v1/users - Phase 25&apos;s new admin-only listing endpoint. Click a row for profile detail and role
        management.
      </p>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Display name</th>
              <th>Status</th>
              <th>Locale</th>
              <th>Created</th>
            </tr>
          </thead>
          <tbody>
            {result.items.length === 0 && (
              <tr>
                <td colSpan={4} className="empty">
                  No users.
                </td>
              </tr>
            )}
            {result.items.map((u) => (
              <tr key={u.id}>
                <td>
                  <Link href={`/users/${u.id}`}>{u.displayName}</Link>
                </td>
                <td>
                  <span className={`badge ${u.status === "active" ? "ok" : "neutral"}`}>{u.status}</span>
                </td>
                <td>{u.locale}</td>
                <td>{new Date(u.createdAt).toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {result.nextCursor && (
        <div style={{ marginTop: 14 }}>
          <Link className="btn small" href={`/users?cursor=${encodeURIComponent(result.nextCursor)}`}>
            Next page →
          </Link>
        </div>
      )}
    </div>
  );
}
