# Platform module

Owns the **platform** capability boundary. Product applications consume this module through contracts/SDKs, never by reaching into its data store.

Includes the Application Registry (`backend/core-api/internal/applications`) - the platform's own record of which applications exist, exposed at `/v1/apps`. See that package's README for detail.

Also includes multi-tenancy (`backend/core-api/internal/tenants`) - `Tenant`/`Membership`, exposed at `/v1/tenants*`. Consumer apps can ignore it; SaaS applications create many tenants per application. See that package's README for detail.
