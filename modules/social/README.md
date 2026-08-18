# Social module

Owns the **social** capability boundary. Product applications consume this module through contracts/SDKs, never by reaching into its data store.

Implemented in `backend/core-api/internal/relationships` - a generic relationship graph (`request`/`accept`/`decline`/`remove`/`block`). The platform never hardcodes what a relationship means ("friend", "follower", "partner" are all just product-supplied `type` strings), only the lifecycle. Exposed at `/v1/relationships*`. See that package's README for detail.

Also includes generic grouping (`backend/core-api/internal/groups`) - friend circles, families, teams, communities, workspaces, gaming guilds are all the same underlying `Group`/`GroupMember` shape. Exposed at `/v1/groups*`. See that package's README for detail.
