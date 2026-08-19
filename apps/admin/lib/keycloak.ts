import "server-only";

const KEYCLOAK_URL = process.env.KEYCLOAK_URL || "http://localhost:8081";
const KEYCLOAK_REALM = process.env.KEYCLOAK_REALM || "core";
const KEYCLOAK_CLIENT_ID = process.env.KEYCLOAK_CLIENT_ID || "core-platform";

export type LoginResult =
  | { ok: true; accessToken: string; expiresIn: number }
  | { ok: false; error: string };

/**
 * A direct Resource Owner Password Credentials grant against the real
 * Keycloak realm every other phase's live validation has used all
 * session long - the same auth boundary as the API Console's "get a
 * token" helper (docs/control), just from a server action instead of
 * client-side JS. This is the right tradeoff for an internal admin
 * portal with a small, trusted user base; a public-facing app would use
 * the Authorization Code + PKCE flow instead, which needs a redirect
 * URI, a callback route, and (for silent refresh) more session-
 * management machinery than this phase's scope calls for.
 */
export async function login(username: string, password: string): Promise<LoginResult> {
  const res = await fetch(`${KEYCLOAK_URL}/realms/${KEYCLOAK_REALM}/protocol/openid-connect/token`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({
      grant_type: "password",
      client_id: KEYCLOAK_CLIENT_ID,
      username,
      password,
    }),
    cache: "no-store",
  });
  const body = await res.json();
  if (!res.ok || !body.access_token) {
    return { ok: false, error: body.error_description || body.error || `HTTP ${res.status}` };
  }
  return { ok: true, accessToken: body.access_token, expiresIn: body.expires_in };
}
