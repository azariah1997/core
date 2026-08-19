"use client";

import { useActionState } from "react";
import { assignRoleAction, revokeRoleAction, type RoleActionState } from "./actions";

const KNOWN_ROLES = ["platform.admin", "support", "moderator"];

function RevokeButton({ userId, role }: { userId: string; role: string }) {
  const action = revokeRoleAction.bind(null, userId);
  const [state, formAction, pending] = useActionState<RoleActionState, FormData>(action, {});
  return (
    <form action={formAction} style={{ display: "inline" }}>
      <input type="hidden" name="role" value={role} />
      <button className="btn small danger" type="submit" disabled={pending}>
        {pending ? "…" : "Revoke"}
      </button>
      {state?.error && <div className="error-box" style={{ marginTop: 8 }}>{state.error}</div>}
    </form>
  );
}

export function RoleManager({ userId, roles }: { userId: string; roles: string[] }) {
  const action = assignRoleAction.bind(null, userId);
  const [state, formAction, pending] = useActionState<RoleActionState, FormData>(action, {});

  return (
    <div className="card">
      <h2>Roles</h2>
      <p className="subtitle">
        Live via POST /v1/authz/roles and /v1/authz/roles/revoke - Phase 25 gave authz its first HTTP surface, replacing
        the throwaway Go programs every earlier phase&apos;s live validation needed to grant itself a role.
      </p>
      {roles.length === 0 ? (
        <p style={{ color: "var(--muted)" }}>No roles assigned.</p>
      ) : (
        <table style={{ marginBottom: 16 }}>
          <tbody>
            {roles.map((r) => (
              <tr key={r}>
                <td style={{ padding: "6px 0" }}>
                  <span className="badge neutral mono">{r}</span>
                </td>
                <td style={{ padding: "6px 0", textAlign: "right" }}>
                  <RevokeButton userId={userId} role={r} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <form action={formAction} className="row">
        <select
          name="role"
          defaultValue=""
          style={{
            flex: 1,
            padding: "9px 12px",
            borderRadius: 7,
            border: "1px solid var(--panel-border)",
            background: "#0e1428",
            color: "var(--text)",
            fontSize: 13.5,
          }}
        >
          <option value="" disabled>
            Grant a role…
          </option>
          {KNOWN_ROLES.filter((r) => !roles.includes(r)).map((r) => (
            <option key={r} value={r}>
              {r}
            </option>
          ))}
        </select>
        <button className="btn primary small" type="submit" disabled={pending}>
          {pending ? "Granting…" : "Grant"}
        </button>
      </form>
      {state?.error && <div className="error-box" style={{ marginTop: 10 }}>{state.error}</div>}
    </div>
  );
}
