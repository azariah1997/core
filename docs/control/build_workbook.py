#!/usr/bin/env python3
"""Builds docs/control/data.xlsx - the Core Platform control-room workbook.

Run from anywhere: python3 docs/control/build_workbook.py
Edit the row data below, re-run, then reload docs/control/index.html.
"""
import base64
import os
import re
import openpyxl
from openpyxl.styles import Font, PatternFill, Alignment, Border, Side
from openpyxl.utils import get_column_letter
from openpyxl.worksheet.table import Table, TableStyleInfo

HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(HERE, "data.xlsx")
HTML = os.path.join(HERE, "index.html")

HEADER_FILL = PatternFill("solid", fgColor="1F2937")
HEADER_FONT = Font(color="FFFFFF", bold=True, size=11)
DONE_FILL = PatternFill("solid", fgColor="D1FAE5")
PENDING_FILL = PatternFill("solid", fgColor="FEF3C7")
NEXT_FILL = PatternFill("solid", fgColor="DBEAFE")
THIN = Side(style="thin", color="D1D5DB")
BORDER = Border(left=THIN, right=THIN, top=THIN, bottom=THIN)
WRAP = Alignment(wrap_text=True, vertical="top")
WRAP_C = Alignment(wrap_text=True, vertical="top", horizontal="center")

wb = openpyxl.Workbook()
wb.remove(wb.active)


def add_sheet(name, headers, rows, widths, freeze="A2", row_height=None):
    ws = wb.create_sheet(name)
    ws.append(headers)
    for c in range(1, len(headers) + 1):
        cell = ws.cell(row=1, column=c)
        cell.font = HEADER_FONT
        cell.fill = HEADER_FILL
        cell.alignment = WRAP_C
        cell.border = BORDER
    for r, row in enumerate(rows, start=2):
        ws.append(row)
        for c in range(1, len(headers) + 1):
            cell = ws.cell(row=r, column=c)
            cell.border = BORDER
            cell.alignment = WRAP
        if row_height:
            ws.row_dimensions[r].height = row_height
    for i, w in enumerate(widths, start=1):
        ws.column_dimensions[get_column_letter(i)].width = w
    ws.freeze_panes = freeze
    last_col = get_column_letter(len(headers))
    last_row = len(rows) + 1
    tbl = Table(displayName=name.replace(" ", "").replace("&", "").replace("-", "")[:30], ref=f"A1:{last_col}{last_row}")
    tbl.tableStyleInfo = TableStyleInfo(name="TableStyleMedium2", showRowStripes=True)
    ws.add_table(tbl)
    return ws


# ---------------------------------------------------------------------------
# 1. OVERVIEW
# ---------------------------------------------------------------------------
ws = wb.create_sheet("Overview")
ws.sheet_view.showGridLines = False
ws["A1"] = "Core Platform - Control Room"
ws["A1"].font = Font(size=20, bold=True, color="1F2937")
ws["A2"] = "A generic, multi-product backend platform (Go services + Next.js admin + Flutter mobile + Terraform/Helm/Docker infra), built phase-by-phase against a 30-phase roadmap."
ws["A2"].font = Font(size=11, italic=True, color="4B5563")
ws["A2"].alignment = Alignment(wrap_text=True)
ws.merge_cells("A2:F2")
ws.row_dimensions[2].height = 32

stats = [
    ("Phases complete", "20 / 30", "67%"),
    ("Backend services", "3", "core-api, realtime-gateway, worker"),
    ("Shared Go packages", "1", "packages/go/platformkit"),
    ("Domain modules (core-api)", "20", "one per completed phase"),
    ("Live HTTP endpoints", "110", "see 'API Endpoints' sheet"),
    ("DB migrations", "18", "data/migrations/0001-0018"),
    ("Real infra dependencies", "9", "Postgres, Valkey, Keycloak, OpenFGA, MinIO, OpenSearch, Temporal, Kafka/Redpanda, OTel"),
    ("Commits so far", "23", "see git log"),
]
r = 4
ws.cell(row=r, column=1, value="At a glance").font = Font(size=13, bold=True)
r += 1
for label, val, note in stats:
    ws.cell(row=r, column=1, value=label).font = Font(bold=True)
    ws.cell(row=r, column=2, value=val).font = Font(size=12, color="047857", bold=True)
    ws.cell(row=r, column=3, value=note).font = Font(italic=True, color="6B7280")
    r += 1

r += 1
ws.cell(row=r, column=1, value="How to read this workbook").font = Font(size=13, bold=True)
r += 1
notes = [
    "Roadmap - all 30 phases, one row each, status + what it delivers.",
    "Todo Checklist - granular task list (completed sub-items and still-to-do sub-items) so progress is trackable at a finer grain than 'phase done or not'.",
    "Modules - every backend/core-api/internal module: what it owns, its storage, its access rule.",
    "API Endpoints - every live HTTP route across all three services, with auth level.",
    "How To Test - copy-paste steps to bring the stack up locally and exercise it for real (mint a token, call endpoints, open a WebSocket).",
    "Infra & Credentials - every local service, its port, and its local-only credentials.",
]
for n in notes:
    ws.cell(row=r, column=1, value="• " + n).alignment = Alignment(wrap_text=True)
    ws.merge_cells(start_row=r, start_column=1, end_row=r, end_column=6)
    r += 1

ws.column_dimensions["A"].width = 28
ws.column_dimensions["B"].width = 16
ws.column_dimensions["C"].width = 70

# ---------------------------------------------------------------------------
# 2. ROADMAP
# ---------------------------------------------------------------------------
roadmap_rows = [
    (1, "Done", "Platform Foundation", "config/logging/errors/health/correlation IDs/OTel/graceful shutdown - the shared platformkit every later phase builds on.", "9b9a65a"),
    (2, "Done", "Application Registry", "First real domain module: Application CRUD, cursor pagination, application.created/updated events.", "65a937d"),
    (3, "Done", "Identity", "Provider abstraction over Keycloak (JWT validation + Admin API), platform-owned IdentityID/UserID, identities linkage table.", "987f0cf"),
    (4, "Done", "Users", "Platform person/account, separate from Identity. Soft-delete lifecycle, auto-provision on first login.", "3f0289e"),
    (5, "Done", "Devices / Sessions", "Per-user device tracking, push-token storage, revoke/reactivate, raw tokens never exposed in responses.", "0762cfa"),
    (6, "Done", "Authorization / Access", "authz.Service.Can(subject, action, resource) - RBAC (Postgres) + fine-grained ReBAC (OpenFGA), the one entry point every module uses.", "892db7b"),
    (7, "Done", "Tenants / Organizations", "Tenant + Membership (owner/admin/member), scoped to an Application. Tenant-role checks answered from Membership data, not OpenFGA.", "9040905"),
    (8, "Done", "Relationships / Social Graph", "Generic RelationshipType/Relationship/Status graph: request/accept/decline/remove/block, no fixed 'friend/follower' semantics.", "542604c"),
    (9, "Done", "Groups / Circles", "Generic Group/GroupMember primitive (friend circles, teams, guilds - same shape), free-form product-defined member roles.", "02b5767"),
    (10, "Done", "Realtime Platform", "New realtime-gateway service: hand-rolled WebSocket (RFC6455), Redis-fanned pub/sub for cross-replica delivery, TTL-based presence.", "1eda4b3"),
    (11, "Done", "Messaging", "Conversation/Message/Delivery/ReadState/Reaction/Attachment, realtime delivery via a new shared rtbus package, durable storage in Postgres.", "a199c4c"),
    (12, "Done", "Notification Platform", "Notification/Template/Preference/Delivery across push/email/sms/in_app/realtime; only push is genuinely provider-tested locally (real device-token check).", "95b8e05"),
    (13, "Done", "File / Media Platform", "Presigned upload/download against real MinIO/S3, real MD5 checksum verification, ownership/permissions/retention/purge.", "9a39606"),
    (14, "Done", "Search Platform", "SearchDocument/SearchProvider over OpenSearch; event-driven indexing via a new worker service polling the outbox.", "6a4eafd"),
    (15, "Done", "Background Jobs", "Job/JobStatus/JobAttempt: immediate/scheduled/delayed/recurring/retry/dead-letter, executed by worker, never inline in HTTP handlers.", "d9221a4"),
    (16, "Done", "Workflows", "Temporal abstraction (Core Workflow API/SDK) for long-running, multi-step, compensating, scheduled processes - Temporal never exposed directly.", "59dfe77"),
    (17, "Done", "Feature Flags", "Feature/FeatureRule/FeatureEvaluation targeting app/env/user/tenant/platform/country/%/version, deterministic FNV-1a percentage bucketing.", "86925a5"),
    (18, "Done", "Remote Configuration", "Typed key/value store scoped by (AppID, Environment, Key), full change-audit trail, config.updated/deleted events.", "9a0ad7b"),
    (19, "Done", "Audit", "Central Audit Service: actor/action/resource/timestamp/correlation/app/tenant/device/metadata. Immutable by omission AND a DB trigger.", "2629e91"),
    (20, "Done", "Privacy", "Consent (append-only), preferences, retention policy, and cross-module ExportUserData/DeleteUserData via an Exporter/Deleter registry (users/devices/files/audit-export-only). Coordinated by a Temporal workflow core-api runs its own embedded worker for, since worker (separate module) can't reach core-api's internal packages.", "44299b2"),
    (21, "Next up", "Trust & Safety", "Reusable Block/Mute/Report/ModerationCase/Suspension/Ban/Appeal, plus rate limiting, spam protection, abuse signals. Product-specific report reasons pluggable.", "-"),
    (22, "Pending", "Billing / Entitlements", "Separate Payment from Entitlement - products ask 'has entitlement X', never 'is the Stripe subscription active'. Provider abstraction first (Stripe, Apple IAP, Google Play).", "-"),
    (23, "Pending", "Analytics", "Generic analytics event envelope (event_name/user_id/anonymous_id/app_id/session_id/timestamp/properties/context). Operational DBs must not become analytics DBs - pipeline foundation for ClickHouse/warehouse/data lake.", "-"),
    (24, "Pending", "AI Gateway", "Provider-neutral interface (OpenAI/Anthropic/Google/local models/Ollama/vLLM) with model routing, quotas, token/cost tracking, audit, timeouts, fallback. Products never call AI vendors directly.", "-"),
    (25, "Pending", "Admin Portal", "The control-plane UI (apps/admin, Next.js) - visibility into apps/users/sessions/devices/roles/tenants/relationships/messages/notifications/files/jobs/flags/config/moderation/audit/health/deployments, always through service APIs, never direct DB queries.", "-"),
    (26, "Pending", "Backstage", "Integrate Backstage as the developer portal - every service/module exposes metadata (owner, lifecycle, repo, docs, dependencies, APIs, events, DB ownership, dashboards, runbooks). Backstage becomes the map of the platform.", "-"),
    (27, "Pending", "SDKs", "Flutter SDK, TypeScript SDK, Go SDK - auth, token refresh, API calls, errors, pagination, safe retries, realtime connection, correlation IDs, device registration. Future products talk to Core primarily through these.", "-"),
    (28, "Pending", "Observability Completion", "Every production component gets structured logs, metrics, traces, health checks, dashboards, alerts. Standard dashboards for API latency, error rate, req/sec, DB connections, Kafka lag, WS connections, Redis latency, notification/job failures.", "-"),
    (29, "Pending", "Platform Control Plane", "One view of applications/services/versions/environments/dependencies/deployments/DB ownership/events/API contracts/health/alerts/recent changes - every module discoverable from one place.", "-"),
    (30, "Pending", "AI Development Context", "Machine-readable context for AI agents - every module carries README, ownership metadata, dependencies, API contract, event contracts, DB ownership, so an agent (like this one) can safely work on any part of the platform.", "-"),
]
ws2 = add_sheet(
    "Roadmap",
    ["Phase", "Status", "Name", "What it delivers", "Commit"],
    roadmap_rows,
    widths=[7, 10, 26, 95, 10],
    row_height=46,
)
for r in range(2, len(roadmap_rows) + 2):
    status = ws2.cell(row=r, column=2).value
    fill = DONE_FILL if status == "Done" else (NEXT_FILL if status == "Next up" else PENDING_FILL)
    ws2.cell(row=r, column=2).fill = fill
    ws2.cell(row=r, column=2).alignment = WRAP_C
    ws2.cell(row=r, column=1).alignment = WRAP_C

# ---------------------------------------------------------------------------
# 3. TODO CHECKLIST (granular)
# ---------------------------------------------------------------------------
todo_rows = []


def done(phase, task, note=""):
    todo_rows.append((phase, "Done", task, note))


def pending(phase, task, note=""):
    todo_rows.append((phase, "Pending", task, note))


for n, name in [
    (1, "Platform Foundation"), (2, "Application Registry"), (3, "Identity"), (4, "Users"),
    (5, "Devices / Sessions"), (6, "Authorization / Access"), (7, "Tenants / Organizations"),
    (8, "Relationships / Social Graph"), (9, "Groups / Circles"), (10, "Realtime Platform"),
    (11, "Messaging"), (12, "Notification Platform"), (13, "File / Media Platform"),
    (14, "Search Platform"), (15, "Background Jobs"), (16, "Workflows"), (17, "Feature Flags"),
    (18, "Remote Configuration"), (19, "Audit"), (20, "Privacy"),
]:
    done(n, f"Phase {n}: {name} implemented, unit-tested, live-validated, documented, committed")

# Extra granular items worth tracking separately (known, documented gaps / deferred sub-scopes)
done(1, "Repo health baseline (make doctor / validate-all / go test / go build) run and passed")
done(2, "Environment / ApplicationVersion / ApplicationConfiguration", "Deferred - no consumer needs them yet, documented as intentional in applications/README.md")
done(5, "Separate Session table", "Deferred - Device.SessionStatus covers today's single-session-per-device need, documented in devices/README.md")
pending(9, "UserPreferences (named in Phase 4, no fields ever specified)", "Deferred to Phase 12+; notification preferences shipped, a generic preferences bag did not")
pending(9, "apps/mobile test/ directory (make flutter-test currently fails)", "Known gap since Phase 9's audit, not yet picked up")
pending(9, "apps/admin next@15 / sharp transitive advisories", "Fix is a Next.js major-version bump - separate, reviewable change")
done(20, "Privacy: consent (append-only history)")
done(20, "Privacy: user data export (ExportUserData registry - users/devices/files/audit)")
done(20, "Privacy: user deletion (DeleteUserData registry - users/devices/files; audit deliberately excluded)")
done(20, "Privacy: retention policy (declared, not scheduled - same documented gap as Phase 13's PurgeExpired)")
done(20, "Privacy: privacy preferences")
done(20, "Privacy: coordinate deletion through workflows/events - core-api's own embedded Temporal worker")
pending(20, "Privacy: notifications module as an Export/Delete participant", "Deliberately deferred to prove the registry makes this a 2-line addition later, not a framework change")
pending(20, "Privacy: relationships/groups/messaging as participants", "Deferred - shared multi-user data needs a redaction strategy this phase's per-user-in-isolation registry doesn't fit")

for n, name, items in [
    (21, "Trust & Safety", ["Block", "Mute", "Report", "ModerationCase", "Suspension", "Ban", "Appeal", "rate limiting", "spam protection", "abuse signals", "pluggable product-specific report reasons"]),
    (22, "Billing / Entitlements", ["Payment model", "Entitlement model (kept separate from Payment)", "provider abstraction (implement first)", "Stripe adapter", "Apple IAP adapter", "Google Play adapter"]),
    (23, "Analytics", ["generic analytics event envelope (event_name/user_id/anonymous_id/app_id/session_id/timestamp/properties/context)", "keep operational DBs out of the analytics path", "pipeline foundation for ClickHouse/warehouse/data lake"]),
    (24, "AI Gateway", ["provider-neutral interface (OpenAI/Anthropic/Google/local/Ollama/vLLM)", "model routing", "quotas", "token usage tracking", "cost tracking", "audit integration", "prompt/version metadata", "timeouts", "fallback"]),
    (25, "Admin Portal", ["control-plane UI in apps/admin", "visibility: Applications", "visibility: Users/Sessions/Devices", "visibility: Roles/Permissions", "visibility: Tenants/Relationships", "visibility: Messages/Notifications", "visibility: Files/Jobs", "visibility: Feature flags/Configuration", "visibility: Moderation/Audit", "visibility: System health/Deployments", "must go through service APIs, never direct DB queries"]),
    (26, "Backstage", ["integrate Backstage as developer portal", "per-module metadata: name/description/owner/lifecycle", "per-module metadata: repository/documentation/dependencies", "per-module metadata: APIs/events/DB ownership/dashboards/runbooks"]),
    (27, "SDKs", ["Flutter SDK", "TypeScript SDK", "Go SDK", "auth + token refresh in each SDK", "pagination + safe retries", "realtime connection helper", "correlation ID propagation", "device registration helper"]),
    (28, "Observability Completion", ["structured logs everywhere", "metrics everywhere", "distributed traces everywhere", "health checks everywhere", "dashboard: API latency", "dashboard: error rate", "dashboard: requests/sec", "dashboard: DB connections", "dashboard: Kafka lag", "dashboard: WebSocket connections", "dashboard: Redis latency", "dashboard: notification failures", "dashboard: background job failures", "alerting rules"]),
    (29, "Platform Control Plane", ["one view: applications/services/versions", "one view: environments/dependencies/deployments", "one view: DB ownership/events/API contracts", "one view: health/alerts/recent changes"]),
    (30, "AI Development Context", ["every module: README", "every module: ownership metadata", "every module: dependencies", "every module: API contract", "every module: event contracts", "every module: DB ownership"]),
]:
    for item in items:
        pending(n, f"{name}: {item}")

ws3 = add_sheet(
    "Todo Checklist",
    ["Phase", "Status", "Task", "Note"],
    todo_rows,
    widths=[7, 10, 70, 60],
    row_height=None,
)
for r in range(2, len(todo_rows) + 2):
    status = ws3.cell(row=r, column=2).value
    ws3.cell(row=r, column=2).fill = DONE_FILL if status == "Done" else PENDING_FILL
    ws3.cell(row=r, column=2).alignment = WRAP_C
    ws3.cell(row=r, column=1).alignment = WRAP_C

# ---------------------------------------------------------------------------
# 4. MODULES
# ---------------------------------------------------------------------------
module_rows = [
    ("applications", "core-api", "Application CRUD, the platform's own top-level tenant-of-tenants concept", "applications", "Open (read) / unauthenticated create in this phase's scope", "Phase 2"),
    ("identity", "core-api", "Auth provider abstraction (Keycloak today), token validation, identity<->user linkage", "identities", "Bearer token required for /me", "Phase 3"),
    ("users", "core-api", "Platform person/account, profile, soft-delete lifecycle", "users", "Self, or self-or-platform.admin for cross-user GET", "Phase 4"),
    ("devices", "core-api", "Per-user device + push-token registry, revoke/reactivate", "devices", "Self only", "Phase 5"),
    ("authz", "core-api", "Can(subject, action, resource) - RBAC + OpenFGA ReBAC, the one authorization entry point", "user_roles + OpenFGA store", "Self-service (HasRole) / role changes are the caller's own privileged action", "Phase 6"),
    ("tenants", "core-api", "Tenant + Membership (owner/admin/member), scoped to an Application", "tenants, tenant_memberships", "Member-only reads, owner/admin-only writes", "Phase 7"),
    ("relationships", "core-api", "Generic relationship graph: request/accept/decline/remove/block", "relationships", "Participant-only", "Phase 8"),
    ("groups", "core-api", "Generic Group/GroupMember, free-form roles, IsManager structural bit", "groups, group_members", "Member-only reads, manager-only writes", "Phase 9"),
    ("realtime-gateway (service)", "realtime-gateway", "WebSocket connections, channels, presence, direct/broadcast delivery", "none (Redis/Valkey only)", "Bearer token required at handshake", "Phase 10"),
    ("messaging", "core-api", "Conversation/Message/Delivery/ReadState/Reaction/Attachment", "conversations, conversation_members, messages, message_*", "Conversation-member-only", "Phase 11"),
    ("notifications", "core-api", "Notification/Template/Preference/Delivery across 5 channels", "notifications, notification_requests, notification_preferences", "Self-or-platform.admin to send; templates are platform.admin-only", "Phase 12"),
    ("files", "core-api", "Presigned upload/download, ownership, retention, checksum", "files", "Owner-or-platform.admin, visibility-scoped downloads", "Phase 13"),
    ("search", "core-api", "SearchDocument/SearchProvider over OpenSearch", "none (OpenSearch only)", "Open query; admin-only manual index/delete", "Phase 14"),
    ("worker: indexer", "worker", "Event-driven OpenSearch indexer, polls outbox_events", "reads outbox_events", "n/a (background)", "Phase 14"),
    ("jobs", "core-api", "Job/JobStatus/JobAttempt - immediate/scheduled/delayed/recurring/retry/dead-letter", "jobs, job_attempts", "Owner-or-platform.admin", "Phase 15"),
    ("worker: jobrunner", "worker", "Claims and executes due jobs (Echo, Webhook handlers)", "reads/writes jobs, job_attempts", "n/a (background)", "Phase 15"),
    ("workflows", "core-api", "Core Workflow API/SDK over Temporal (Start/Describe/Signal only)", "workflow_runs (ownership only - state lives in Temporal)", "Owner-or-platform.admin", "Phase 16"),
    ("worker: workflows", "worker", "ApprovalWorkflow, PingWebhookWorkflow (Temporal worker)", "none (Temporal-native state)", "n/a (background)", "Phase 16"),
    ("features", "core-api", "Feature/FeatureRule/FeatureEvaluation, deterministic percentage rollout", "features, feature_rules", "Open evaluate; platform.admin-only manage", "Phase 17"),
    ("remoteconfig", "core-api", "Typed KV config store, full audit trail per change", "config_entries, config_changes", "Open read; platform.admin-only write/history", "Phase 18"),
    ("audit", "core-api", "Central, immutable audit trail (API-omission + DB trigger enforced)", "audit_events", "Open to record; platform.admin-only to read", "Phase 19"),
    ("privacy", "core-api", "Consent, preferences, retention policy, cross-module export/delete via a registry", "privacy_consents, privacy_preferences, retention_policies, data_export_requests, data_deletion_requests", "Self for own data; platform.admin for retention policies", "Phase 20"),
    ("privacy: embedded Temporal worker", "core-api", "Runs privacy's own export/deletion workflows in-process (task queue privacy-tasks)", "none (Temporal-native state)", "n/a (background, inside core-api's own process)", "Phase 20"),
]
ws4 = add_sheet(
    "Modules",
    ["Module", "Service", "Responsibility", "Storage", "Access model", "Introduced"],
    module_rows,
    widths=[22, 16, 58, 32, 42, 11],
    row_height=32,
)

# ---------------------------------------------------------------------------
# 5. API ENDPOINTS
# ---------------------------------------------------------------------------
def E(method, path, module, auth, note=""):
    return (method, path, module, auth, note)


endpoint_rows = [
    E("GET", "/livez", "platform", "Open", "process alive"),
    E("GET", "/readyz", "platform", "Open", "dependencies ready"),
    E("GET", "/healthz", "platform", "Open", "aggregated health"),
    E("GET", "/v1/platform", "platform", "Open", "platform name/env/version"),
    E("POST", "/v1/apps", "applications", "Open (this phase's scope)", ""),
    E("GET", "/v1/apps", "applications", "Open", "cursor-paginated list"),
    E("GET", "/v1/apps/{id}", "applications", "Open", ""),
    E("PATCH", "/v1/apps/{id}", "applications", "Open (this phase's scope)", ""),
    E("GET", "/v1/identity/me", "identity", "Authenticated", "resolves the caller's Identity"),
    E("GET", "/v1/users/me", "users", "Authenticated (self)", ""),
    E("PATCH", "/v1/users/me", "users", "Authenticated (self)", ""),
    E("GET", "/v1/users/{id}", "users", "Self or platform.admin", ""),
    E("GET", "/v1/users/me/devices", "devices", "Authenticated (self)", ""),
    E("POST", "/v1/users/me/devices", "devices", "Authenticated (self)", "register/upsert"),
    E("DELETE", "/v1/users/me/devices/{id}", "devices", "Authenticated (self)", "soft-revoke"),
    E("GET", "/v1/tenants", "tenants", "Authenticated", "scoped to caller's own tenants"),
    E("POST", "/v1/tenants", "tenants", "Authenticated", "creator becomes owner"),
    E("GET", "/v1/tenants/{id}", "tenants", "Member", ""),
    E("PATCH", "/v1/tenants/{id}", "tenants", "Owner or admin", ""),
    E("GET", "/v1/tenants/{id}/members", "tenants", "Member", ""),
    E("POST", "/v1/tenants/{id}/members", "tenants", "Owner or admin", ""),
    E("DELETE", "/v1/tenants/{id}/members/{userId}", "tenants", "Owner/admin, or self (leave)", ""),
    E("GET", "/v1/relationships", "relationships", "Participant", ""),
    E("GET", "/v1/relationships/{id}", "relationships", "Participant", ""),
    E("POST", "/v1/relationships", "relationships", "Authenticated", "request"),
    E("POST", "/v1/relationships/block", "relationships", "Authenticated", ""),
    E("POST", "/v1/relationships/{id}/accept", "relationships", "Target only", ""),
    E("POST", "/v1/relationships/{id}/decline", "relationships", "Target only", ""),
    E("DELETE", "/v1/relationships/{id}", "relationships", "Participant", "remove/unfriend"),
    E("GET", "/v1/groups", "groups", "Member", ""),
    E("POST", "/v1/groups", "groups", "Authenticated", "creator becomes manager"),
    E("GET", "/v1/groups/{id}", "groups", "Member", ""),
    E("PATCH", "/v1/groups/{id}", "groups", "Manager", ""),
    E("GET", "/v1/groups/{id}/members", "groups", "Member", ""),
    E("POST", "/v1/groups/{id}/members", "groups", "Manager", ""),
    E("PATCH", "/v1/groups/{id}/members/{userId}", "groups", "Manager", "promote/demote IsManager"),
    E("DELETE", "/v1/groups/{id}/members/{userId}", "groups", "Manager, or self (leave)", ""),
    E("WS", "/ws (realtime-gateway)", "realtime-gateway", "Bearer token at handshake", "subscribe/publish/broadcast/direct message"),
    E("GET", "/v1/conversations", "messaging", "Member", ""),
    E("POST", "/v1/conversations", "messaging", "Authenticated", "direct convos dedup automatically"),
    E("GET", "/v1/conversations/{id}", "messaging", "Member", ""),
    E("GET", "/v1/conversations/{id}/members", "messaging", "Member", ""),
    E("POST", "/v1/conversations/{id}/members", "messaging", "Member (group only)", ""),
    E("DELETE", "/v1/conversations/{id}/members/{userId}", "messaging", "Self only (leave)", ""),
    E("GET", "/v1/conversations/{id}/messages", "messaging", "Member", "cursor pagination"),
    E("POST", "/v1/conversations/{id}/messages", "messaging", "Member", "pushes over realtime too"),
    E("GET", "/v1/conversations/{id}/messages/{messageId}", "messaging", "Member", ""),
    E("POST", "/v1/conversations/{id}/messages/{messageId}/delivered", "messaging", "Member", "idempotent ack"),
    E("GET", "/v1/conversations/{id}/messages/{messageId}/reactions", "messaging", "Member", ""),
    E("POST", "/v1/conversations/{id}/messages/{messageId}/reactions", "messaging", "Member", "idempotent add"),
    E("DELETE", "/v1/conversations/{id}/messages/{messageId}/reactions/{type}", "messaging", "Member", ""),
    E("GET", "/v1/conversations/{id}/read/{userId}", "messaging", "Member", ""),
    E("PUT", "/v1/conversations/{id}/read", "messaging", "Member", ""),
    E("GET", "/v1/notifications", "notifications", "Authenticated (self)", ""),
    E("POST", "/v1/notifications", "notifications", "Self or platform.admin", "send"),
    E("GET", "/v1/notifications/{id}", "notifications", "Self or platform.admin", ""),
    E("GET", "/v1/notifications/{id}/deliveries", "notifications", "Self or platform.admin", ""),
    E("POST", "/v1/notifications/{id}/deliveries/{deliveryId}/retry", "notifications", "Self or platform.admin", ""),
    E("GET", "/v1/notification-preferences", "notifications", "Authenticated (self)", ""),
    E("PUT", "/v1/notification-preferences", "notifications", "Authenticated (self)", "category opt-outs"),
    E("GET", "/v1/notification-preferences/quiet-hours", "notifications", "Authenticated (self)", ""),
    E("PUT", "/v1/notification-preferences/quiet-hours", "notifications", "Authenticated (self)", ""),
    E("POST", "/v1/notification-templates", "notifications", "platform.admin", ""),
    E("GET", "/v1/notification-templates/{appId}/{key}", "notifications", "platform.admin", ""),
    E("PATCH", "/v1/notification-templates/{appId}/{key}", "notifications", "platform.admin", ""),
    E("GET", "/v1/files", "files", "Owner or platform.admin", ""),
    E("POST", "/v1/files", "files", "Authenticated", "requests a presigned upload URL"),
    E("POST", "/v1/files/{id}/confirm", "files", "Owner", "verifies real size + MD5 checksum"),
    E("GET", "/v1/files/{id}", "files", "Owner, or public if visibility allows", ""),
    E("GET", "/v1/files/{id}/download", "files", "Owner, or public if visibility allows", "presigned GET"),
    E("DELETE", "/v1/files/{id}", "files", "Owner or platform.admin", "also deletes from storage"),
    E("POST", "/v1/files/purge-expired", "files", "platform.admin", ""),
    E("GET", "/v1/search", "search", "Open", ""),
    E("POST", "/v1/search/index", "search", "platform.admin", "manual index (normally event-driven)"),
    E("DELETE", "/v1/search/index/{type}/{id}", "search", "platform.admin", ""),
    E("GET", "/v1/jobs", "jobs", "Owner or platform.admin", ""),
    E("POST", "/v1/jobs", "jobs", "Authenticated", "enqueue only - never runs inline"),
    E("GET", "/v1/jobs/{id}", "jobs", "Owner or platform.admin", ""),
    E("GET", "/v1/jobs/{id}/attempts", "jobs", "Owner or platform.admin", ""),
    E("POST", "/v1/workflows", "workflows", "Authenticated", "starts a Temporal workflow"),
    E("GET", "/v1/workflows/{id}", "workflows", "Owner or platform.admin", "live-queried from Temporal"),
    E("POST", "/v1/workflows/{id}/signal", "workflows", "Owner or platform.admin", ""),
    E("GET", "/v1/features", "features", "platform.admin", ""),
    E("POST", "/v1/features", "features", "platform.admin", ""),
    E("GET", "/v1/features/{appId}/{key}", "features", "platform.admin", ""),
    E("PATCH", "/v1/features/{appId}/{key}", "features", "platform.admin", "kill switch etc"),
    E("POST", "/v1/features/{appId}/{key}/evaluate", "features", "Open", "isEnabled(feature, context)"),
    E("GET", "/v1/features/{appId}/{key}/rules", "features", "platform.admin", ""),
    E("POST", "/v1/features/{appId}/{key}/rules", "features", "platform.admin", ""),
    E("PATCH", "/v1/features/{appId}/{key}/rules/{ruleId}", "features", "platform.admin", ""),
    E("DELETE", "/v1/features/{appId}/{key}/rules/{ruleId}", "features", "platform.admin", ""),
    E("GET", "/v1/config", "remoteconfig", "Open", "list entries"),
    E("GET", "/v1/config/{appId}/{environment}/{key}", "remoteconfig", "Open", ""),
    E("PUT", "/v1/config/{appId}/{environment}/{key}", "remoteconfig", "platform.admin", "writes an audited Change row"),
    E("DELETE", "/v1/config/{appId}/{environment}/{key}", "remoteconfig", "platform.admin", ""),
    E("GET", "/v1/config/{appId}/{environment}/{key}/history", "remoteconfig", "platform.admin", ""),
    E("POST", "/v1/audit", "audit", "Authenticated (actor forced server-side)", ""),
    E("GET", "/v1/audit", "audit", "platform.admin", ""),
    E("GET", "/v1/audit/{id}", "audit", "platform.admin", ""),
    E("POST", "/v1/privacy/consent", "privacy", "Authenticated (self)", "append-only history"),
    E("GET", "/v1/privacy/consent", "privacy", "Authenticated (self)", ""),
    E("GET", "/v1/privacy/preferences", "privacy", "Authenticated (self)", ""),
    E("PUT", "/v1/privacy/preferences", "privacy", "Authenticated (self)", ""),
    E("POST", "/v1/privacy/retention-policies", "privacy", "platform.admin", ""),
    E("GET", "/v1/privacy/retention-policies", "privacy", "platform.admin", ""),
    E("POST", "/v1/privacy/export", "privacy", "Authenticated (self)", "starts a Temporal workflow"),
    E("GET", "/v1/privacy/export/{id}", "privacy", "Self or platform.admin", "status is read from Postgres, not Temporal"),
    E("GET", "/v1/privacy/export/{id}/download", "privacy", "Self or platform.admin", "presigned MinIO/S3 GET"),
    E("POST", "/v1/privacy/delete", "privacy", "Authenticated (self)", "starts a Temporal workflow"),
    E("GET", "/v1/privacy/delete/{id}", "privacy", "Self or platform.admin", "a self-deleted user must use an admin to check this"),
]
ws5 = add_sheet(
    "API Endpoints",
    ["Method", "Path", "Module", "Auth", "Notes"],
    endpoint_rows,
    widths=[8, 46, 16, 32, 42],
)

# ---------------------------------------------------------------------------
# 6. HOW TO TEST
# ---------------------------------------------------------------------------
test_rows = [
    ("1", "Bring up local infra", "make local-up", "Starts Postgres, Valkey, Keycloak, OpenFGA, MinIO, OpenSearch, Temporal, Kafka/Redpanda + the observability stack (13 containers). Idempotent, one command."),
    ("2", "Check everything is healthy", "docker ps", "Every container should show 'Up' (Postgres shows '(healthy)'). Re-run make local-up if anything is missing - Keycloak/OpenFGA have no persistent volume, so realm/store data is re-imported on every restart."),
    ("3", "Build the services", "make build", "Compiles core-api, realtime-gateway, worker, and platformkit. Should exit 0 with no output."),
    ("4", "Run the test suite", "make test", "Runs every module's unit tests against in-memory fakes. Should be all 'ok'."),
    ("5", "Start core-api", "set -a && source .env && set +a && go run ./backend/core-api/cmd/server", "Listens on :8080 (see Infra sheet). Watch its stdout for structured JSON logs."),
    ("6", "Start realtime-gateway (optional)", "set -a && source .env && set +a && go run ./backend/realtime-gateway/cmd/server", "Listens on :8090. Only needed to test WebSocket/presence features."),
    ("7", "Start worker (optional)", "set -a && source .env && set +a && go run ./backend/worker/cmd/worker", "Runs the job runner, search indexer, and Temporal workflow worker. Only needed to test jobs/search/workflows end to end."),
    ("8", "Confirm the API is up", "curl -s http://localhost:8080/healthz", "Should return {\"status\":\"ok\",...} with every dependency 'ok': true."),
    ("9", "Mint a real login token", "curl -s -X POST http://localhost:8081/realms/core/protocol/openid-connect/token -d grant_type=password -d client_id=core-platform -d username=demo -d password=demo | python3 -c \"import sys,json;print(json.load(sys.stdin)['access_token'])\"", "The realm ships one seeded user, 'demo' / 'demo'. Save the output as $TOKEN - it's a real JWT good for 300 seconds."),
    ("10", "Call an authenticated endpoint", "curl -s http://localhost:8080/v1/users/me -H \"Authorization: Bearer $TOKEN\"", "First call auto-provisions a platform User linked to the Keycloak identity. Re-run it - you get back the same user."),
    ("11", "Create a second, non-admin test user", "Use Keycloak's Admin API (admin/admin against the master realm) to POST a user to /admin/realms/core/users - see the Audit phase's live validation for the exact curl sequence.", "Needed for anything that checks cross-user permissions (403-for-a-stranger tests)."),
    ("12", "Grant platform.admin to a user", "There is no HTTP route for this by design (it's a privileged, rarely-needed action) - write a small throwaway Go program under backend/core-api/cmd/<name>/main.go that constructs authz.Service the same way cmd/server/main.go does and calls AssignRole, then delete it.", "This is the same pattern used to verify every admin-gated module during development - see any phase's entry in VALIDATION.md for a worked example."),
    ("13", "Exercise a module end-to-end", "e.g. POST /v1/apps, then GET /v1/apps, then GET /v1/apps/{id} with the returned id", "Pick any row from the 'API Endpoints' sheet and try it - every route takes/returns plain JSON. Look at each module's own README.md (backend/core-api/internal/<module>/README.md) for the exact request/response shapes and the scoping decisions behind them."),
    ("14", "Watch a WebSocket live", "wscat -c \"ws://localhost:8090/ws?access_token=$TOKEN\" (or any RFC6455 client)", "You should receive a {\"type\":\"connected\",...} frame immediately. Subscribe to a channel, then publish from a second connection to see fan-out."),
    ("15", "Watch the database directly", "docker exec -it docker-postgres-1 psql -U core -d core", "Useful for sanity-checking: SELECT * FROM outbox_events ORDER BY id DESC LIMIT 20; or SELECT * FROM audit_events ORDER BY occurred_at DESC LIMIT 20;"),
    ("16", "Read the validation history", "VALIDATION.md (repo root)", "Every phase's actual live-validation pass is written up there in detail - exact curl sequences, real bugs found and fixed, and what was deliberately left out of scope."),
    ("17", "Run the smoke test", "make smoke", "A scripted end-to-end pass (scripts/smoke.sh) against the real running services - the fastest single command to confirm the whole stack still works after a change."),
]
ws6 = add_sheet(
    "How To Test",
    ["Step", "What", "Command / Where", "Notes"],
    test_rows,
    widths=[6, 30, 70, 60],
    row_height=48,
)

# ---------------------------------------------------------------------------
# 7. INFRA & CREDENTIALS
# ---------------------------------------------------------------------------
infra_rows = [
    ("core-api", "Go service", "http://localhost:8080", "The main platform API - almost every module in this workbook lives here.", "n/a"),
    ("realtime-gateway", "Go service", "ws://localhost:8090", "WebSocket connections, presence, channel fan-out.", "n/a"),
    ("worker", "Go service", "http://localhost:8091 (health only)", "Background job runner, search indexer, Temporal workflow worker.", "n/a"),
    ("Postgres", "Docker container", "localhost:5432", "System of record for every domain module.", "core / core, db 'core'"),
    ("Valkey (Redis-compatible)", "Docker container", "localhost:6379", "Realtime pub/sub fan-out, presence (TTL-based).", "no auth locally"),
    ("Keycloak", "Docker container", "http://localhost:8081", "Identity provider. Realm 'core' is re-imported on every restart (no persistent volume).", "admin/admin (master realm); demo/demo (core realm, seeded user)"),
    ("OpenFGA", "Docker container", "http://localhost:8082", "Fine-grained ReBAC engine behind authz.Service.Can. In-memory store, self-heals its store/model on every restart.", "no auth locally"),
    ("MinIO (S3-compatible)", "Docker container", "http://localhost:9000", "Object storage backing the files module.", "minio / minio123, bucket 'core-platform'"),
    ("OpenSearch", "Docker container", "http://localhost:9200", "Backs the search module, indexed by worker's indexer.", "no auth locally"),
    ("Temporal", "Docker container", "localhost:7233", "Durable workflow execution engine behind the workflows module.", "no auth locally"),
    ("Kafka / Redpanda", "Docker container", "localhost:9092", "Health-checked today; no producer wired yet (outbox-to-Kafka relay is a documented future gap).", "no auth locally"),
    ("OTel Collector / Tempo / Prometheus / Grafana / Loki", "Docker containers", "see infra/docker/docker-compose.yml", "Full observability stack - traces, metrics, logs, dashboards.", "no auth locally"),
]
ws7 = add_sheet(
    "Infra & Credentials",
    ["Service", "Type", "Address", "Purpose", "Local credentials"],
    infra_rows,
    widths=[36, 16, 34, 55, 40],
    row_height=32,
)

for name in wb.sheetnames:
    if name != "Overview":
        wb[name].sheet_view.showGridLines = False

wb.move_sheet("Overview", offset=-len(wb.sheetnames) + 1)
wb.save(OUT)
print("wrote", OUT)

# Bake a base64 copy of the workbook into index.html so the page also works
# when opened directly (file://), where browsers block fetch("data.xlsx").
# index.html always prefers the live file when served over http; this is
# only the offline fallback.
with open(OUT, "rb") as f:
    b64 = base64.b64encode(f.read()).decode("ascii")

with open(HTML, "r", encoding="utf-8") as f:
    html = f.read()

pattern = r'const EMBEDDED_XLSX_B64 = "[^"]*";'
replacement = f'const EMBEDDED_XLSX_B64 = "{b64}";'
new_html, n = re.subn(pattern, replacement, html)
if n != 1:
    raise SystemExit(f"expected exactly one EMBEDDED_XLSX_B64 placeholder in {HTML}, found {n}")

with open(HTML, "w", encoding="utf-8") as f:
    f.write(new_html)
print(f"embedded {len(b64)} base64 chars into {HTML}")
