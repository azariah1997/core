"use client";

import { useActionState } from "react";
import { loginAction, type LoginState } from "./actions";

const initialState: LoginState = {};

export default function LoginPage() {
  const [state, formAction, pending] = useActionState(loginAction, initialState);

  return (
    <div className="login-wrap">
      <div className="card login-card">
        <h1>Core Platform</h1>
        <p className="subtitle">Sign in with your platform account (Keycloak realm &quot;core&quot;).</p>
        {state?.error && <div className="error-box">{state.error}</div>}
        <form action={formAction}>
          <div className="field">
            <label htmlFor="username">Username</label>
            <input id="username" name="username" type="text" autoComplete="username" defaultValue="demo" required />
          </div>
          <div className="field">
            <label htmlFor="password">Password</label>
            <input id="password" name="password" type="password" autoComplete="current-password" required />
          </div>
          <button className="btn primary" type="submit" disabled={pending} style={{ width: "100%" }}>
            {pending ? "Signing in…" : "Sign in"}
          </button>
        </form>
        <p className="subtitle" style={{ marginTop: 16, marginBottom: 0 }}>
          Only platform.admin accounts see admin actions - everyone with a valid account can sign in and browse read-only views their role permits.
        </p>
      </div>
    </div>
  );
}
