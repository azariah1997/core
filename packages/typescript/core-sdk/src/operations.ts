import { CoreClient } from "./client.js";
import type { Page, PageFetcher } from "./pagination.js";

/**
 * Typed convenience methods layered on CoreClient.request. Covers the
 * same representative "core identity" slice the Go SDK does (platform,
 * identity, users, devices, applications) plus everything the real
 * Admin Portal (apps/admin, Phase 25) needs - the strongest evidence a
 * TypeScript SDK actually works is a real product using it for
 * everything, not a parallel hand-rolled client sitting next to it
 * (see apps/admin/lib/api.ts's own history for what this replaced).
 * Every other route is still reachable via the inherited request()
 * method; hand-typing more wrappers is real, additive work anyone can
 * do the same way these were written (copy a *http.go response
 * struct's JSON shape verbatim - never guessed), not a framework
 * limitation.
 */
export class CoreApi extends CoreClient {
  // --- platform / identity ---

  getPlatform() {
    return this.request<Platform>("GET", "/v1/platform");
  }

  identityMe() {
    return this.request<Identity>("GET", "/v1/identity/me");
  }

  // --- users ---

  usersMe() {
    return this.request<User>("GET", "/v1/users/me");
  }

  usersUpdateMe(input: UpdateUserInput) {
    return this.request<User>("PATCH", "/v1/users/me", input);
  }

  usersGet(id: string) {
    return this.request<User>("GET", `/v1/users/${id}`);
  }

  /** platform.admin-only (Phase 25's admin-wide listing). */
  usersList(cursor?: string) {
    return this.request<Page<User>>("GET", "/v1/users", undefined, { query: { cursor } });
  }

  // --- devices ---

  /** The roadmap names "device registration" as its own SDK
   * responsibility, distinct from generic "API calls," since a real-time
   * connection (see RealtimeClient.dial) needs a registered device id
   * first. */
  devicesRegister(input: RegisterDeviceInput) {
    return this.request<Device>("POST", "/v1/users/me/devices", input);
  }

  async devicesList() {
    const res = await this.request<{ items: Device[] }>("GET", "/v1/users/me/devices");
    return res.items;
  }

  devicesRevoke(id: string) {
    return this.request<void>("DELETE", `/v1/users/me/devices/${id}`);
  }

  // --- applications ---

  applicationsCreate(input: CreateApplicationInput) {
    return this.request<Application>("POST", "/v1/apps", input);
  }

  applicationsList(cursor?: string) {
    return this.request<Page<Application>>("GET", "/v1/apps", undefined, { query: { cursor } });
  }

  applicationsGet(id: string) {
    return this.request<Application>("GET", `/v1/apps/${id}`);
  }

  // --- roles (authz, Phase 25's HTTP surface) ---

  rolesFor(userId: string) {
    return this.request<{ userId: string; roles: string[] | null }>("GET", "/v1/authz/roles", undefined, { query: { userId } });
  }

  assignRole(userId: string, role: string) {
    return this.request<{ userId: string; roles: string[] }>("POST", "/v1/authz/roles", { userId, role });
  }

  revokeRole(userId: string, role: string) {
    return this.request<{ userId: string; roles: string[] | null }>("POST", "/v1/authz/roles/revoke", { userId, role });
  }

  // --- audit ---

  audit(limit = 50) {
    return this.request<Page<AuditRecord>>("GET", "/v1/audit", undefined, { query: { limit: String(limit) } });
  }

  // --- moderation (trust & safety) ---

  async moderationCases() {
    const res = await this.request<{ items: ModerationCase[] }>("GET", "/v1/trustsafety/cases");
    return res.items;
  }

  // --- feature flags ---

  async features() {
    const res = await this.request<{ items: Feature[] }>("GET", "/v1/features");
    return res.items;
  }

  // --- jobs ---

  async jobs() {
    const res = await this.request<{ items: Job[] }>("GET", "/v1/jobs");
    return res.items;
  }

  // --- billing ---

  async entitlements(userId?: string) {
    const res = await this.request<{ items: Entitlement[] }>("GET", "/v1/billing/entitlements", undefined, { query: { userId } });
    return res.items;
  }

  // --- AI gateway ---

  async aiUsage(userId?: string) {
    const res = await this.request<{ items: AiCompletion[] }>("GET", "/v1/ai/usage", undefined, { query: { userId } });
    return res.items;
  }

  // --- remote config ---

  async config(appId: string, environment: string) {
    const res = await this.request<{ items: ConfigEntry[] }>("GET", "/v1/config", undefined, { query: { appId, environment } });
    return res.items;
  }
}

/** Adapts a CoreApi list method (userId?/cursor? -> Page<T>) to the
 * generic PageFetcher shape paginate()/collectAll() expect. */
export function pageFetcherFrom<T>(list: (cursor?: string) => Promise<Page<T>>): PageFetcher<T> {
  return (cursor) => list(cursor);
}

// --- response/request shapes, each copied verbatim from the owning
// module's real *http.go response struct - never guessed. ---

export interface Platform {
  name: string;
  environment: string;
  apiVersion: string;
}

export interface Identity {
  id: string;
  provider: string;
  providerSubject: string;
  status: string;
  userId?: string;
}

export interface User {
  id: string;
  displayName: string;
  avatarRef?: string;
  locale: string;
  timezone: string;
  status: string;
  createdAt: string;
  updatedAt: string;
}

export interface UpdateUserInput {
  displayName?: string;
  avatarRef?: string;
  locale?: string;
  timezone?: string;
  status?: string;
}

export interface Device {
  id: string;
  clientDeviceId: string;
  platform: string;
  osVersion?: string;
  appVersion?: string;
  locale: string;
  timezone: string;
  hasPushToken: boolean;
  sessionStatus: string;
  lastActiveAt: string;
  createdAt: string;
}

export interface RegisterDeviceInput {
  clientDeviceId: string;
  platform: string;
  osVersion?: string;
  appVersion?: string;
  locale?: string;
  timezone?: string;
  pushToken?: string;
}

export interface Application {
  id: string;
  slug: string;
  name: string;
  status: string;
  createdAt: string;
  updatedAt: string;
}

export interface CreateApplicationInput {
  slug: string;
  name: string;
}

export interface AuditRecord {
  id: string;
  actorUserId?: string;
  action: string;
  resourceType: string;
  resourceId: string;
  correlationId?: string;
  metadata?: Record<string, unknown>;
  occurredAt: string;
}

export interface ModerationCase {
  id: string;
  resourceType: string;
  resourceId: string;
  status: string;
  assignedTo?: string;
  resolution?: string;
  reportCount: number;
  createdAt: string;
  resolvedAt?: string;
}

export interface Feature {
  id: string;
  appId: string;
  key: string;
  description?: string;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface Job {
  id: string;
  type: string;
  status: string;
  scheduledFor?: string;
  attempts: number;
  maxAttempts: number;
  createdAt: string;
  updatedAt: string;
}

export interface Entitlement {
  id: string;
  key: string;
  source: string;
  grantedAt: string;
  expiresAt?: string;
  revokedAt?: string;
  active: boolean;
}

export interface AiCompletion {
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
}

export interface ConfigEntry {
  id: string;
  appId: string;
  environment: string;
  key: string;
  value: unknown;
  description?: string;
  updatedBy?: string;
  createdAt: string;
  updatedAt: string;
}
