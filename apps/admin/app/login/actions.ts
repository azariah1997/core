"use server";

import { redirect } from "next/navigation";
import { login } from "../../lib/keycloak";
import { setSession } from "../../lib/session";

const CORE_API_URL = process.env.CORE_API_URL || "http://localhost:8080";

export type LoginState = { error?: string };

export async function loginAction(_prev: LoginState, formData: FormData): Promise<LoginState> {
  const username = String(formData.get("username") || "");
  const password = String(formData.get("password") || "");
  if (!username || !password) {
    return { error: "Username and password are required." };
  }

  const result = await login(username, password);
  if (!result.ok) {
    return { error: result.error };
  }

  // Resolve the platform user this identity maps to (auto-provisioning
  // on first login, the same EnsureForIdentity path every other client
  // of this API goes through) so the sidebar can show a real name
  // instead of the raw Keycloak username.
  const meRes = await fetch(`${CORE_API_URL}/v1/users/me`, {
    headers: { Authorization: `Bearer ${result.accessToken}` },
    cache: "no-store",
  });
  if (!meRes.ok) {
    return { error: "Logged in to Keycloak, but core-api rejected the token - is it running?" };
  }
  const me = await meRes.json();

  await setSession({
    accessToken: result.accessToken,
    expiresAt: Date.now() + result.expiresIn * 1000,
    userId: me.id,
    displayName: me.displayName,
  });

  redirect("/");
}
