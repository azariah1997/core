import Link from "next/link";
import { getUser, listRoles, listEntitlements, listAiUsage, ApiError } from "../../../../lib/api";
import { ApiErrorBox } from "../../../../components/ApiErrorBox";
import { RoleManager } from "./RoleManager";

export const dynamic = "force-dynamic";

export default async function UserDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;

  let user;
  try {
    user = await getUser(id);
  } catch (e) {
    return (
      <div>
        <p>
          <Link href="/users">← Users</Link>
        </p>
        <ApiErrorBox error={e} />
      </div>
    );
  }

  // Roles, entitlements, and AI usage are each independently optional -
  // a caller without platform.admin can still view the profile itself
  // (self-or-admin, Phase 6's original rule) even if the roles panel
  // 403s; that's shown as its own error, not a page-wide failure.
  const [rolesResult, entitlementsResult, aiUsageResult] = await Promise.allSettled([
    listRoles(id),
    listEntitlements(id),
    listAiUsage(id),
  ]);

  return (
    <div className="stack" style={{ gap: 20 }}>
      <p>
        <Link href="/users">← Users</Link>
      </p>
      <div className="card">
        <div className="row" style={{ justifyContent: "space-between" }}>
          <h1 style={{ margin: 0 }}>{user.displayName}</h1>
          <span className={`badge ${user.status === "active" ? "ok" : "neutral"}`}>{user.status}</span>
        </div>
        <div style={{ marginTop: 14 }}>
          <div className="kv">
            <span className="mono" style={{ color: "var(--muted)" }}>
              id
            </span>
            <span className="mono">{user.id}</span>
          </div>
          <div className="kv">
            <span style={{ color: "var(--muted)" }}>Locale</span>
            <span>{user.locale}</span>
          </div>
          <div className="kv">
            <span style={{ color: "var(--muted)" }}>Timezone</span>
            <span>{user.timezone}</span>
          </div>
          <div className="kv">
            <span style={{ color: "var(--muted)" }}>Created</span>
            <span>{new Date(user.createdAt).toLocaleString()}</span>
          </div>
          <div className="kv">
            <span style={{ color: "var(--muted)" }}>Updated</span>
            <span>{new Date(user.updatedAt).toLocaleString()}</span>
          </div>
        </div>
      </div>

      {rolesResult.status === "fulfilled" ? (
        <RoleManager userId={id} roles={rolesResult.value.roles || []} />
      ) : (
        <div className="card">
          <h2>Roles</h2>
          <ApiErrorBox error={rolesResult.reason} />
        </div>
      )}

      <div className="card">
        <h2>Entitlements</h2>
        <p className="subtitle">Live from GET /v1/billing/entitlements.</p>
        {entitlementsResult.status === "fulfilled" ? (
          entitlementsResult.value.items.length === 0 ? (
            <p style={{ color: "var(--muted)" }}>None.</p>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Key</th>
                  <th>Source</th>
                  <th>Active</th>
                </tr>
              </thead>
              <tbody>
                {entitlementsResult.value.items.map((e) => (
                  <tr key={e.id}>
                    <td className="mono">{e.key}</td>
                    <td className="mono" style={{ color: "var(--muted)" }}>
                      {e.source}
                    </td>
                    <td>
                      <span className={`badge ${e.active ? "ok" : "neutral"}`}>{e.active ? "active" : "inactive"}</span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )
        ) : (
          <ApiErrorBox error={entitlementsResult.reason} />
        )}
      </div>

      <div className="card">
        <h2>AI Gateway usage</h2>
        <p className="subtitle">Live from GET /v1/ai/usage.</p>
        {aiUsageResult.status === "fulfilled" ? (
          aiUsageResult.value.items.length === 0 ? (
            <p style={{ color: "var(--muted)" }}>None.</p>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Model alias</th>
                  <th>Tokens</th>
                  <th>Cost</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {aiUsageResult.value.items.map((c) => (
                  <tr key={c.id}>
                    <td className="mono">{c.modelAlias}</td>
                    <td>{c.totalTokens}</td>
                    <td>{c.costCents}¢</td>
                    <td>
                      <span className={`badge ${c.status === "completed" ? "ok" : "err"}`}>{c.status}</span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )
        ) : (
          <ApiErrorBox error={aiUsageResult.reason} />
        )}
      </div>
    </div>
  );
}

// ApiError is re-exported purely so this file type-checks against
// Promise.allSettled's rejection shape without importing it unused.
export type { ApiError };
