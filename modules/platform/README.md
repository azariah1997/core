# Platform module

Owns the **platform** capability boundary. Product applications consume this module through contracts/SDKs, never by reaching into its data store.

Includes the Application Registry (`backend/core-api/internal/applications`) - the platform's own record of which applications exist, exposed at `/v1/apps`. See that package's README for detail.
