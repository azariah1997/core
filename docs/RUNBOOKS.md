# Runbooks

Real operational procedures for this local/dev environment, written from incidents actually hit during this platform's build-out (see `VALIDATION.md` for the full history). Linked from the Backstage catalog's `core-platform` System entity (`catalog/system.yaml`).

## Bring up local infrastructure

```sh
make local-up
```

Starts Postgres, Valkey, Keycloak, OpenFGA, MinIO, OpenSearch, Temporal, Kafka/Redpanda, Ollama, and the observability stack. Idempotent - safe to re-run. Confirm with `docker ps`: every container should show `Up` (Postgres shows `(healthy)`).

## Recover platform.admin after a Keycloak restart

**Symptom:** a previously-working admin account suddenly gets 403s on admin-only routes (`GET /v1/users`, `POST /v1/authz/roles`, etc.) after `make local-up` or a container restart.

**Why:** Keycloak's dev-mode storage has no persistent volume - restarting the container wipes every user except the ones statically imported from `infra/keycloak/realm-core.json`. A fresh login re-provisions a *new* platform User (via `EnsureForIdentity`) linked to the new Keycloak subject, which starts with zero roles - the role grant lived on the old, now-orphaned platform User row.

**Fix:**
1. Log in again to get a fresh token for the affected account.
2. Find the new platform User's id: `GET /v1/users/me` with that token.
3. Grant the role via the real HTTP route (Phase 25 - no more throwaway Go programs needed for this):
   ```sh
   curl -s -X POST http://localhost:8080/v1/authz/roles \
     -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
     -d '{"userId":"<new-user-id>","role":"platform.admin"}'
   ```
   `$ADMIN_TOKEN` must belong to a caller who already has `platform.admin` - see "Bootstrap the first admin" below if there isn't one.

## Bootstrap the first platform.admin in a brand-new environment

`authz.Service.AssignRole` has no caller-privilege check at the service layer (only the HTTP route does, via `requireAdmin`) - so the very first admin grant in an environment with zero admins can't go through the API. Write a small throwaway program under `backend/core-api/cmd/<name>/main.go` that constructs `authz.Service` the same way `cmd/server/main.go` does and calls `AssignRole` directly, run it once with `go run`, then delete it. This is the same pattern used to bootstrap every admin-gated module during this platform's own development - see any phase's entry in `VALIDATION.md` for a worked example.

## Rotate the Stripe webhook secret

Update `STRIPE_WEBHOOK_SECRET` in `.env`, restart `core-api`. `billing/stripe`'s signature verification reads it at startup (`platformkit/config`), so there's no live-reload - a restart is required. Old signatures signed with the previous secret will correctly start failing verification immediately after restart; this is expected, not a bug.

## Regenerate `contracts/openapi/core-api.yaml`'s auto-generated paths

If a module's `http.go` adds, removes, or changes a route: `python3 scripts/gen-openapi-paths.py` prints OpenAPI path items for every registered route not already documented, derived from the real `mux.Handle(...)` calls and each handler's actual `http.Status*` code - not guessed. Paste the output above the `components:` line in `contracts/openapi/core-api.yaml`, then run `python3 scripts/embed-catalog-api-defs.py` (Backstage's catalog needs the spec embedded directly - see that script's own header for why `$text` doesn't work here) and `python3 scripts/validate_contracts.py`.

## Run the Admin Portal or Backstage locally

Admin Portal: `cd apps/admin && npm run dev` - see `apps/admin/README.md`.

Backstage: `cd platform/backstage && yarn start` - needs Node 20/22 and Yarn (this repo's primary toolchain uses whatever `node`/`npm` is on `PATH`; Backstage specifically needs a real Yarn install and a Node version it supports - see `platform/backstage/README.md` if the versions on `PATH` don't work for it).
