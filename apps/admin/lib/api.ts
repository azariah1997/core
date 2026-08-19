import "server-only";
import { CoreApi, StaticTokenSource, ApiError as SdkApiError } from "@core-platform/sdk";
import { getSession } from "./session";

const CORE_API_URL = process.env.CORE_API_URL || "http://localhost:8080";
const REALTIME_URL = process.env.REALTIME_HEALTH_URL || "http://localhost:8090";
const WORKER_URL = process.env.WORKER_HEALTH_URL || "http://localhost:8091";

export class ApiError extends Error {
  status: number;
  code?: string;
  constructor(status: number, message: string, code?: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

/**
 * Phase 27's real payoff for this app: every call below now goes
 * through @core-platform/sdk (packages/typescript/core-sdk) instead of
 * a hand-rolled fetch wrapper - the same auth/retry/error-decoding
 * logic every future product gets, exercised here by a real one. The
 * session's raw Keycloak JWT becomes a StaticTokenSource (this app
 * already holds a live token per request; the SDK's own
 * PasswordTokenSource is for a caller that needs to mint one itself).
 * "Do not bypass service APIs by querying production databases
 * directly" (the roadmap's own instruction for Phase 25) is still
 * enforced structurally: this app has no database credentials
 * configured at all.
 */
async function api(): Promise<CoreApi> {
  const session = await getSession();
  if (!session) {
    throw new ApiError(401, "no active session");
  }
  return new CoreApi(CORE_API_URL, { tokenSource: new StaticTokenSource(session.accessToken) });
}

async function withApi<T>(fn: (client: CoreApi) => Promise<T>): Promise<T> {
  const client = await api();
  try {
    return await fn(client);
  } catch (err) {
    if (err instanceof SdkApiError) {
      throw new ApiError(err.statusCode, err.message, err.code);
    }
    throw err;
  }
}

// --- health (Phase 25's "System health" visibility) ---
// Unauthenticated pings across three different base URLs, deliberately
// not routed through the SDK (which is for authenticated core-api/
// realtime-gateway API calls, not raw infra health checks).

export type HealthResult = { name: string; url: string; ok: boolean; detail?: string };

async function checkHealth(name: string, baseUrl: string): Promise<HealthResult> {
  try {
    const res = await fetch(`${baseUrl}/healthz`, { cache: "no-store", signal: AbortSignal.timeout(3000) });
    const body = await res.json().catch(() => ({}));
    return { name, url: baseUrl, ok: res.ok, detail: body?.status };
  } catch (e) {
    return { name, url: baseUrl, ok: false, detail: e instanceof Error ? e.message : "unreachable" };
  }
}

export async function getSystemHealth(): Promise<HealthResult[]> {
  return Promise.all([
    checkHealth("core-api", CORE_API_URL),
    checkHealth("realtime-gateway", REALTIME_URL),
    checkHealth("worker", WORKER_URL),
  ]);
}

// --- applications ---

export type { Application } from "@core-platform/sdk";

export function listApplications() {
  return withApi((client) => client.applicationsList());
}

// --- users (Phase 25's admin-only list) ---

export type { User as PlatformUser } from "@core-platform/sdk";

export function listUsers(cursor?: string) {
  return withApi((client) => client.usersList(cursor));
}

export function getUser(id: string) {
  return withApi((client) => client.usersGet(id));
}

// --- roles (Phase 25's authz HTTP surface) ---

export function listRoles(userId: string) {
  return withApi((client) => client.rolesFor(userId));
}

// assignRole/revokeRole are plain async functions, not Server Actions
// themselves - server-only already keeps this whole module out of any
// client bundle. The actual "use server" boundary a <form> can submit
// to lives in each page's own actions.ts, which calls these.
export function assignRole(userId: string, role: string) {
  return withApi((client) => client.assignRole(userId, role));
}

export function revokeRole(userId: string, role: string) {
  return withApi((client) => client.revokeRole(userId, role));
}

// --- audit ---

export type { AuditRecord } from "@core-platform/sdk";

export function listAudit(limit = 50) {
  return withApi((client) => client.audit(limit));
}

// --- moderation (trust & safety) ---

export type { ModerationCase } from "@core-platform/sdk";

export function listModerationCases() {
  return withApi(async (client) => ({ items: await client.moderationCases() }));
}

// --- feature flags ---

export type { Feature } from "@core-platform/sdk";

export function listFeatures() {
  return withApi(async (client) => ({ items: await client.features() }));
}

// --- jobs ---

export type { Job } from "@core-platform/sdk";

export function listJobs() {
  return withApi(async (client) => ({ items: await client.jobs() }));
}

// --- billing (bonus visibility, not one of the 18 named areas but real and cheap to add) ---

export type { Entitlement } from "@core-platform/sdk";

export function listEntitlements(userId: string) {
  return withApi(async (client) => ({ items: await client.entitlements(userId) }));
}

// --- AI gateway usage (bonus visibility) ---

export type { AiCompletion } from "@core-platform/sdk";

export function listAiUsage(userId: string) {
  return withApi(async (client) => ({ items: await client.aiUsage(userId) }));
}

// --- remote config ---

export type { ConfigEntry } from "@core-platform/sdk";

export function listConfig(appId: string, environment: string) {
  return withApi(async (client) => ({ items: await client.config(appId, environment) }));
}
