# Access module

Owns the **access** capability boundary. Product applications consume this module through contracts/SDKs, never by reaching into its data store.

Implemented in `backend/core-api/internal/authz` - RBAC roles (`platform.admin`, `support`, `moderator`) plus a fine-grained, OpenFGA-backed `Can(subject, action, resource)` check. Every other domain module depends on this instead of implementing permission logic independently, per the platform's non-negotiable rule. See that package's README for detail.
