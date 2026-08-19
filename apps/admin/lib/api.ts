import "server-only";
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
 * The one place this app talks to core-api - every page in this portal
 * goes through here, which always attaches the real session's Bearer
 * token. "Do not bypass service APIs by querying production databases
 * directly" (the roadmap's own instruction for this phase) is enforced
 * structurally: this app has no database credentials configured at all,
 * so there is nothing to bypass service APIs with even if a page tried.
 */
async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const session = await getSession();
  if (!session) {
    throw new ApiError(401, "no active session");
  }
  const res = await fetch(`${CORE_API_URL}${path}`, {
    ...init,
    headers: {
      Authorization: `Bearer ${session.accessToken}`,
      "Content-Type": "application/json",
      ...(init?.headers || {}),
    },
    cache: "no-store",
  });
  const text = await res.text();
  const body = text ? JSON.parse(text) : null;
  if (!res.ok) {
    throw new ApiError(res.status, body?.message || `HTTP ${res.status}`, body?.code);
  }
  return body as T;
}

// --- health (Phase 25's "System health" visibility) ---

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

export type Application = {
  id: string;
  slug: string;
  name: string;
  status: string;
  createdAt: string;
  updatedAt: string;
};

export function listApplications() {
  return apiFetch<{ items: Application[] }>("/v1/apps");
}

// --- users (Phase 25's new admin-only list) ---

export type PlatformUser = {
  id: string;
  displayName: string;
  avatarRef?: string;
  locale: string;
  timezone: string;
  status: string;
  createdAt: string;
  updatedAt: string;
};

export function listUsers(cursor?: string) {
  const qs = cursor ? `?cursor=${encodeURIComponent(cursor)}` : "";
  return apiFetch<{ items: PlatformUser[]; nextCursor: string }>(`/v1/users${qs}`);
}

export function getUser(id: string) {
  return apiFetch<PlatformUser>(`/v1/users/${id}`);
}

// --- roles (Phase 25's new authz HTTP surface) ---

export function listRoles(userId: string) {
  return apiFetch<{ userId: string; roles: string[] | null }>(`/v1/authz/roles?userId=${encodeURIComponent(userId)}`);
}

// assignRole/revokeRole are plain async functions, not Server Actions
// themselves - server-only already keeps this whole module out of any
// client bundle. The actual "use server" boundary a <form> can submit
// to lives in each page's own actions.ts, which calls these.
export function assignRole(userId: string, role: string) {
  return apiFetch<{ userId: string; roles: string[] }>("/v1/authz/roles", {
    method: "POST",
    body: JSON.stringify({ userId, role }),
  });
}

export function revokeRole(userId: string, role: string) {
  return apiFetch<{ userId: string; roles: string[] | null }>("/v1/authz/roles/revoke", {
    method: "POST",
    body: JSON.stringify({ userId, role }),
  });
}

// --- audit ---

export type AuditRecord = {
  id: string;
  actorUserId?: string;
  action: string;
  resourceType: string;
  resourceId: string;
  correlationId?: string;
  metadata?: Record<string, unknown>;
  occurredAt: string;
};

export function listAudit(limit = 50) {
  return apiFetch<{ items: AuditRecord[]; nextCursor: string }>(`/v1/audit?limit=${limit}`);
}

// --- moderation (trust & safety) ---

export type ModerationCase = {
  id: string;
  resourceType: string;
  resourceId: string;
  status: string;
  assignedTo?: string;
  resolution?: string;
  reportCount: number;
  createdAt: string;
  resolvedAt?: string;
};

export function listModerationCases() {
  return apiFetch<{ items: ModerationCase[] }>("/v1/trustsafety/cases");
}

// --- feature flags ---

export type Feature = {
  id: string;
  appId: string;
  key: string;
  description?: string;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
};

export function listFeatures() {
  return apiFetch<{ items: Feature[] }>("/v1/features");
}

// --- jobs ---

export type Job = {
  id: string;
  type: string;
  status: string;
  scheduledFor?: string;
  attempts: number;
  maxAttempts: number;
  createdAt: string;
  updatedAt: string;
};

export function listJobs() {
  return apiFetch<{ items: Job[] }>("/v1/jobs");
}

// --- billing (bonus visibility, not one of the 18 named areas but real and cheap to add) ---

export type Entitlement = {
  id: string;
  key: string;
  source: string;
  grantedAt: string;
  expiresAt?: string;
  revokedAt?: string;
  active: boolean;
};

export function listEntitlements(userId: string) {
  return apiFetch<{ items: Entitlement[] }>(`/v1/billing/entitlements?userId=${encodeURIComponent(userId)}`);
}

// --- AI gateway usage (bonus visibility) ---

export type AiCompletion = {
  id: string;
  modelAlias: string;
  provider?: string;
  model?: string;
  promptKey?: string;
  totalTokens: number;
  costCents: number;
  latencyMs: number;
  status: string;
  createdAt: string;
};

export function listAiUsage(userId: string) {
  return apiFetch<{ items: AiCompletion[] }>(`/v1/ai/usage?userId=${encodeURIComponent(userId)}`);
}

// --- remote config ---

export type ConfigEntry = {
  id: string;
  appId: string;
  environment: string;
  key: string;
  value: unknown;
  description?: string;
  updatedBy?: string;
  createdAt: string;
  updatedAt: string;
};

export function listConfig(appId: string, environment: string) {
  return apiFetch<{ items: ConfigEntry[] }>(
    `/v1/config?appId=${encodeURIComponent(appId)}&environment=${encodeURIComponent(environment)}`
  );
}
