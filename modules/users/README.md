# Users module

Owns the **users** capability boundary. Product applications consume this module through contracts/SDKs, never by reaching into its data store.

Implemented in `backend/core-api/internal/users` - the platform person/account, separate from Identity ("who authenticated", owned by the `identity` module). `GET/PATCH /v1/users/me`, `GET /v1/users/{id}`. A User is auto-provisioned and linked on an Identity's first authenticated request. See that package's README for detail.
