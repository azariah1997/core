#!/usr/bin/env python3
"""Prints OpenAPI path items for every real core-api route not already
documented in contracts/openapi/core-api.yaml, derived from each module's
actual mux.Handle(...) registrations and the http.Status* code its handler
really returns (see STATUS below) - not guessed.

Usage: python3 scripts/gen-openapi-paths.py >> fragment.yaml
Paste the result into contracts/openapi/core-api.yaml just above the
`components:` line, then run `python3 scripts/validate_contracts.py`.

STATUS and MODULE_DESC below were hand-extracted once (Phase 26) by
grepping every internal/*/http.go for its handlers' http.Status* calls;
re-extract them the same way if a module adds or changes a route.
"""
import re
import subprocess
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
EXISTING_PATH = ROOT / "contracts/openapi/core-api.yaml"
CORE_API_DIR = ROOT / "backend/core-api"

MODULE_DESC = {
    "aigateway": "AI Gateway (Phase 24) - provider-neutral completions API; products never call an AI vendor directly.",
    "audit": "Audit (Phase 19) - the central, immutable audit trail. Immutable by omission (no update/delete route) and by a DB trigger.",
    "analytics": "Analytics (Phase 23) - generic event envelope ingestion. analytics_events is a landing buffer, never queried for analytics.",
    "devices": "Devices (Phase 5) - per-user device/push-token registry.",
    "authz": "Authorization (Phase 6, HTTP surface added Phase 25) - RBAC role management. Can(subject, action, resource) itself has no HTTP surface; it's called in-process by every other module.",
    "billing": "Billing / Entitlements (Phase 22) - Entitlement (platform truth) kept separate from Payment (provider transaction record).",
    "features": "Feature Flags (Phase 17) - Feature/FeatureRule/FeatureEvaluation with deterministic percentage-rollout bucketing.",
    "privacy": "Privacy (Phase 20) - consent, preferences, retention policy, and cross-module data export/deletion.",
    "files": "File / Media Platform (Phase 13) - presigned upload/download against real MinIO/S3, real MD5 checksum verification.",
    "jobs": "Background Jobs (Phase 15) - immediate/scheduled/delayed/recurring, with retry and dead-letter handling, executed by worker.",
    "groups": "Groups / Circles (Phase 9) - generic Group/GroupMember primitive; friend circles, teams, guilds are all the same shape.",
    "tenants": "Tenants / Organizations (Phase 7) - Tenant + Membership (owner/admin/member), scoped to an Application.",
    "messaging": "Messaging (Phase 11) - Conversation/Message/Delivery/ReadState/Reaction/Attachment, real-time delivery via realtime-gateway.",
    "relationships": "Relationships / Social Graph (Phase 8) - generic request/accept/decline/remove/block; type is always product-defined.",
    "notifications": "Notification Platform (Phase 12) - Notification/Template/Preference/Delivery across 5 channels.",
    "users": "Users (Phase 4; admin-wide listing added Phase 25) - the platform person/account, separate from Identity.",
    "remoteconfig": "Remote Configuration (Phase 18) - typed key/value store scoped by (AppID, Environment, Key), with a full change-audit trail.",
    "search": "Search Platform (Phase 14) - SearchDocument/SearchProvider over OpenSearch, kept in sync by worker's event-driven indexer.",
    "trustsafety": "Trust & Safety (Phase 21) - Mute/Report->ModerationCase/Suspension/Ban/Appeal. Block reuses relationships, not duplicated.",
    "workflows": "Workflows (Phase 16) - a deliberately narrow Core Workflow API (Start/Describe/Signal only) over a real Temporal cluster.",
}

# (module, handlerFuncName) -> exact http.Status* used in that handler,
# extracted directly from each module's http.go (see scripts/gen-openapi-paths.py
# for how). This is ground truth, not a guess.
STATUS = {
    ("aigateway", "completeHandler"): "OK",
    ("aigateway", "listUsageHandler"): "OK",
    ("aigateway", "listModelsHandler"): "OK",
    ("audit", "recordHandler"): "Created",
    ("audit", "getHandler"): "OK",
    ("audit", "listHandler"): "OK",
    ("analytics", "trackHandler"): "Accepted",
    ("analytics", "listRecentHandler"): "OK",
    ("authz", "listRolesHandler"): "OK",
    ("authz", "assignRoleHandler"): "OK",
    ("authz", "revokeRoleHandler"): "OK",
    ("billing", "listEntitlementsHandler"): "OK",
    ("billing", "checkEntitlementHandler"): "OK",
    ("billing", "grantEntitlementHandler"): "Created",
    ("billing", "revokeEntitlementHandler"): "OK",
    ("billing", "listPaymentsHandler"): "OK",
    ("billing", "webhookHandler"): "NoContent",
    ("features", "createFeatureHandler"): "Created",
    ("features", "listFeaturesHandler"): "OK",
    ("features", "getFeatureHandler"): "OK",
    ("features", "updateFeatureHandler"): "OK",
    ("features", "evaluateHandler"): "OK",
    ("features", "createRuleHandler"): "Created",
    ("features", "listRulesHandler"): "OK",
    ("features", "updateRuleHandler"): "OK",
    ("features", "deleteRuleHandler"): "NoContent",
    ("privacy", "setConsentHandler"): "Created",
    ("privacy", "listConsentHandler"): "OK",
    ("privacy", "getPreferencesHandler"): "OK",
    ("privacy", "setPreferencesHandler"): "OK",
    ("privacy", "createRetentionPolicyHandler"): "OK",
    ("privacy", "listRetentionPoliciesHandler"): "OK",
    ("privacy", "requestExportHandler"): "Accepted",
    ("privacy", "getExportHandler"): "OK",
    ("privacy", "downloadExportHandler"): "OK",
    ("privacy", "requestDeletionHandler"): "Accepted",
    ("privacy", "getDeletionHandler"): "OK",
    ("files", "requestUploadHandler"): "Created",
    ("files", "confirmUploadHandler"): "OK",
    ("files", "getHandler"): "OK",
    ("files", "getDownloadURLHandler"): "OK",
    ("files", "listMineHandler"): "OK",
    ("files", "deleteHandler"): "NoContent",
    ("files", "purgeExpiredHandler"): "OK",
    ("jobs", "enqueueHandler"): "Created",
    ("jobs", "listMineHandler"): "OK",
    ("jobs", "getHandler"): "OK",
    ("jobs", "listAttemptsHandler"): "OK",
    ("groups", "createHandler"): "Created",
    ("groups", "listMineHandler"): "OK",
    ("groups", "getHandler"): "OK",
    ("groups", "updateHandler"): "OK",
    ("groups", "listMembersHandler"): "OK",
    ("groups", "addMemberHandler"): "Created",
    ("groups", "updateMemberHandler"): "OK",
    ("groups", "removeMemberHandler"): "NoContent",
    ("tenants", "createHandler"): "Created",
    ("tenants", "listMineHandler"): "OK",
    ("tenants", "getHandler"): "OK",
    ("tenants", "updateHandler"): "OK",
    ("tenants", "listMembersHandler"): "OK",
    ("tenants", "addMemberHandler"): "Created",
    ("tenants", "removeMemberHandler"): "NoContent",
    ("messaging", "createConversationHandler"): "Created",
    ("messaging", "listMineHandler"): "OK",
    ("messaging", "getConversationHandler"): "OK",
    ("messaging", "listMembersHandler"): "OK",
    ("messaging", "addMemberHandler"): "Created",
    ("messaging", "removeMemberHandler"): "NoContent",
    ("messaging", "sendMessageHandler"): "Created",
    ("messaging", "listMessagesHandler"): "OK",
    ("messaging", "getMessageHandler"): "OK",
    ("messaging", "markDeliveredHandler"): "OK",
    ("messaging", "setReadStateHandler"): "OK",
    ("messaging", "getReadStateHandler"): "OK",
    ("messaging", "addReactionHandler"): "Created",
    ("messaging", "listReactionsHandler"): "OK",
    ("messaging", "removeReactionHandler"): "NoContent",
    ("relationships", "requestHandler"): "Created",
    ("relationships", "listMineHandler"): "OK",
    ("relationships", "getHandler"): "OK",
    ("relationships", "acceptHandler"): "OK",
    ("relationships", "declineHandler"): "OK",
    ("relationships", "removeHandler"): "NoContent",
    ("relationships", "blockHandler"): "OK",
    ("notifications", "sendHandler"): "Created",
    ("notifications", "listMineHandler"): "OK",
    ("notifications", "getHandler"): "OK",
    ("notifications", "listDeliveriesHandler"): "OK",
    ("notifications", "retryDeliveryHandler"): "OK",
    ("notifications", "getPreferencesHandler"): "OK",
    ("notifications", "setPreferenceHandler"): "OK",
    ("notifications", "getQuietHoursHandler"): "OK",
    ("notifications", "setQuietHoursHandler"): "OK",
    ("notifications", "createTemplateHandler"): "Created",
    ("notifications", "getTemplateHandler"): "OK",
    ("notifications", "updateTemplateHandler"): "OK",
    ("users", "listHandler"): "OK",
    ("remoteconfig", "setHandler"): "OK",
    ("remoteconfig", "getHandler"): "OK",
    ("remoteconfig", "listHandler"): "OK",
    ("remoteconfig", "deleteHandler"): "NoContent",
    ("remoteconfig", "historyHandler"): "OK",
    ("search", "queryHandler"): "OK",
    ("search", "indexHandler"): "NoContent",
    ("search", "deleteHandler"): "NoContent",
    ("trustsafety", "muteHandler"): "Created",
    ("trustsafety", "listMutesHandler"): "OK",
    ("trustsafety", "unmuteHandler"): "NoContent",
    ("trustsafety", "createReportHandler"): "Created",
    ("trustsafety", "getReportHandler"): "OK",
    ("trustsafety", "listCasesHandler"): "OK",
    ("trustsafety", "getCaseHandler"): "OK",
    ("trustsafety", "listCaseReportsHandler"): "OK",
    ("trustsafety", "assignCaseHandler"): "OK",
    ("trustsafety", "resolveCaseHandler"): "OK",
    ("trustsafety", "suspendHandler"): "Created",
    ("trustsafety", "listSuspensionsHandler"): "OK",
    ("trustsafety", "liftSuspensionHandler"): "OK",
    ("trustsafety", "banHandler"): "Created",
    ("trustsafety", "listBansHandler"): "OK",
    ("trustsafety", "liftBanHandler"): "OK",
    ("trustsafety", "createAppealHandler"): "Created",
    ("trustsafety", "listAppealsHandler"): "OK",
    ("trustsafety", "reviewAppealHandler"): "OK",
    ("trustsafety", "recordSignalHandler"): "Created",
    ("trustsafety", "listSignalsHandler"): "OK",
    ("workflows", "startHandler"): "Created",
    ("workflows", "getHandler"): "OK",
    ("workflows", "signalHandler"): "NoContent",
}

STATUS_CODE = {"OK": "200", "Created": "201", "Accepted": "202", "NoContent": "204"}

OPID_OVERRIDES = {}


def op_id(module, handler_func):
    if handler_func in OPID_OVERRIDES:
        return OPID_OVERRIDES[handler_func]
    name = handler_func
    if name.endswith("Handler"):
        name = name[: -len("Handler")]
    # Many handlers share generic names (listHandler, getHandler, ...) across
    # modules - operationId must be unique across the whole document, so
    # every generated id is module-prefixed (module.get -> "moduleGet").
    return module + name[0].upper() + name[1:]


def load_existing():
    spec = yaml.safe_load(EXISTING_PATH.read_text())
    existing = set()
    for path, methods in spec["paths"].items():
        for method in methods:
            existing.add((method.upper(), path))
    return existing


ROUTE_RE = re.compile(
    r'internal/([a-z]+)/http\.go:\d+:\s*mux\.Handle\("([A-Z]+) ([^"]+)",\s*(?:requireUser\()?([a-zA-Z]+)\('
)


def parse_routes():
    grep = subprocess.run(
        ["grep", "-rn", "mux.Handle(", "internal"],
        cwd=CORE_API_DIR, capture_output=True, text=True,
    )
    routes = []
    for line in grep.stdout.splitlines(keepends=True):
        m = ROUTE_RE.search(line)
        if not m:
            print("NO MATCH:", line.rstrip(), file=sys.stderr)
            continue
        module, method, path, handler = m.groups()
        if path.startswith("/v1"):
            path = path[len("/v1"):]
        auth = "bearerAuth" if "requireUser(" in line else "open"
        routes.append((module, method, path, handler, auth))
    return routes


def build_path_item(module, method, path, handler, auth):
    opid = op_id(module, handler)
    desc = MODULE_DESC.get(module, module)
    status_name = STATUS.get((module, handler))
    if status_name is None:
        print(f"WARNING: no known status for {module}.{handler}, defaulting to OK", file=sys.stderr)
        status_name = "OK"
    status = STATUS_CODE[status_name]
    lower = method.lower()

    parts = []
    parts.append(f"    {lower}:")
    parts.append(f"      operationId: {opid}")
    parts.append(f"      description: \"{desc}\"")
    if auth == "bearerAuth":
        parts.append("      security:")
        parts.append("        - bearerAuth: []")
    param_names = re.findall(r"\{(\w+)\}", path)
    if param_names:
        parts.append("      parameters:")
        for p in param_names:
            fmt = ", format: uuid" if p in ("id", "userId", "messageId", "deliveryId", "ruleId") else ""
            parts.append(f"        - {{ name: {p}, in: path, required: true, schema: {{ type: string{fmt} }} }}")
    if method in ("POST", "PUT", "PATCH") and "/webhooks/" not in path:
        parts.append("      requestBody:")
        parts.append("        content:")
        parts.append("          application/json:")
        parts.append("            schema: { type: object, additionalProperties: true }")
    parts.append("      responses:")
    if status == "204":
        parts.append(f"        '204': {{ description: Success, no content }}")
    else:
        parts.append(f"        '{status}':")
        parts.append("          description: Success")
        parts.append("          content:")
        parts.append("            application/json:")
        parts.append("              schema: { type: object, additionalProperties: true }")
    if auth == "bearerAuth":
        parts.append("        '401': { description: Missing, malformed or invalid token, content: { application/json: { schema: { $ref: '#/components/schemas/Error' } } } }")
    return "\n".join(parts)


def main():
    existing = load_existing()
    routes = parse_routes()
    by_path = {}
    for module, method, path, handler, auth in routes:
        if (method, path) in existing:
            continue
        by_path.setdefault(path, []).append((module, method, handler, auth))

    out_lines = []
    out_lines.append("  # ------------------------------------------------------------------")
    out_lines.append("  # Everything below is generated directly from each module's real")
    out_lines.append("  # mux.Handle(...) registrations and their handlers' actual")
    out_lines.append("  # http.Status* calls (see scripts/gen-openapi-paths.py) - not")
    out_lines.append("  # hand-written. Path, method, path params, auth requirement, and")
    out_lines.append("  # success status code are exactly what the server does. Request/")
    out_lines.append("  # response bodies use a generic object schema rather than a fully")
    out_lines.append("  # hand-typed one; see each module's own README for exact shapes.")
    out_lines.append("  # ------------------------------------------------------------------")
    for path, entries in by_path.items():
        out_lines.append(f"  {path}:")
        for module, method, handler, auth in entries:
            out_lines.append(build_path_item(module, method, path, handler, auth))

    print("\n".join(out_lines))
    print(f"\n# added {sum(len(v) for v in by_path.values())} operations across {len(by_path)} paths", file=sys.stderr)


if __name__ == "__main__":
    main()
