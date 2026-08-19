/**
 * Returns a valid Bearer token for the current request, refreshing it
 * internally if needed. Every core-api call goes through one -
 * "authentication" and "token refresh" are the SDK's own job, not
 * something every calling product should reimplement.
 */
export interface TokenSource {
  token(): Promise<string>;
}

/** Wraps an already-minted token with no refresh behavior - useful for
 * short-lived scripts, tests, and server-side code that already holds a
 * session token from somewhere else (e.g. a cookie, as apps/admin does). */
export class StaticTokenSource implements TokenSource {
  constructor(private readonly value: string) {}
  async token(): Promise<string> {
    return this.value;
  }
}

export interface PasswordTokenSourceOptions {
  keycloakUrl: string;
  realm: string;
  clientId: string;
  username: string;
  password: string;
  fetchImpl?: typeof fetch;
  /** How long before real expiry a token is treated as already-expired,
   * so a request never races a token dying mid-flight. Default 15s. */
  refreshSkewMs?: number;
}

/**
 * A real Resource Owner Password Credentials grant against Keycloak
 * (the same grant apps/admin's server-side login and every phase's
 * live-validation curl sequence in this repo have used) with
 * transparent refresh - a caller just awaits token() and either gets
 * the cached one or a freshly refreshed one, never an expired one.
 */
export class PasswordTokenSource implements TokenSource {
  private cached?: { token: string; expiresAt: number };
  private inFlight?: Promise<string>;

  constructor(private readonly opts: PasswordTokenSourceOptions) {}

  async token(): Promise<string> {
    const skew = this.opts.refreshSkewMs ?? 15_000;
    if (this.cached && Date.now() + skew < this.cached.expiresAt) {
      return this.cached.token;
    }
    // Coalesce concurrent refreshes into a single in-flight request -
    // several near-simultaneous calls shouldn't each mint a new token.
    if (!this.inFlight) {
      this.inFlight = this.mint().finally(() => {
        this.inFlight = undefined;
      });
    }
    return this.inFlight;
  }

  private async mint(): Promise<string> {
    const fetchImpl = this.opts.fetchImpl ?? fetch;
    const tokenUrl = `${this.opts.keycloakUrl.replace(/\/$/, "")}/realms/${this.opts.realm}/protocol/openid-connect/token`;
    const res = await fetchImpl(tokenUrl, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({
        grant_type: "password",
        client_id: this.opts.clientId,
        username: this.opts.username,
        password: this.opts.password,
      }),
    });
    const body = await res.json();
    if (!res.ok || !body.access_token) {
      throw new Error(`coresdk: token request failed with status ${res.status}: ${body.error_description ?? body.error ?? "unknown error"}`);
    }
    this.cached = { token: body.access_token, expiresAt: Date.now() + body.expires_in * 1000 };
    return this.cached.token;
  }
}
