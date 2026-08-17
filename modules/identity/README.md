# Identity module

Owns the **identity** capability boundary. Product applications consume this module through contracts/SDKs, never by reaching into its data store.

Implemented in `backend/core-api/internal/identity` - a provider-neutral `Provider` interface (token validation plus provider-side identity management) with a real Keycloak-backed implementation, and the platform's own `identities` linkage table (provider + providerSubject -> platform Identity, optionally linked to a User once Phase 4 exists). See that package's README for detail.
