import "server-only";
import { cookies } from "next/headers";

const COOKIE_NAME = "core_admin_session";

export type Session = {
  accessToken: string;
  expiresAt: number; // epoch ms
  userId: string;
  displayName: string;
};

/**
 * The session cookie holds the real Keycloak-issued access token
 * directly - it is already a signed JWT Keycloak's own key produced, so
 * wrapping it in a second signature/encryption layer would protect
 * against a threat (a tampered cookie value) the JWT's own signature
 * already defeats. A production deployment fronting many admins would
 * still want a proper session store (so a token can be revoked
 * server-side before its own expiry) - out of scope for this internal,
 * local-dev-first admin tool, the same "not built speculatively" bar
 * every backend phase in this repo has held itself to.
 */
export async function setSession(session: Session) {
  const store = await cookies();
  store.set(COOKIE_NAME, JSON.stringify(session), {
    httpOnly: true,
    sameSite: "lax",
    secure: process.env.NODE_ENV === "production",
    path: "/",
    expires: new Date(session.expiresAt),
  });
}

export async function getSession(): Promise<Session | null> {
  const store = await cookies();
  const raw = store.get(COOKIE_NAME)?.value;
  if (!raw) return null;
  try {
    const session = JSON.parse(raw) as Session;
    if (session.expiresAt <= Date.now()) return null;
    return session;
  } catch {
    return null;
  }
}

export async function clearSession() {
  const store = await cookies();
  store.delete(COOKIE_NAME);
}
