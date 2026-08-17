# Validation Report

Generated validation for this boilerplate in the creation environment.

## Passed
- Repository/config required-artifact checks
- Secret-pattern sanity check
- OpenAPI/AsyncAPI/Protobuf contract presence checks
- SQL migration/outbox static check
- Argo CD and Helm scaffold static check
- YAML parsing for Compose, observability, Argo, Backstage catalog and API contracts
- `gofmt`
- `go test` across all Go workspace modules
- `go build` across all Go workspace modules
- Live Core API health and platform endpoints
- Live Realtime Gateway health endpoint
- Live RFC6455 WebSocket upgrade handshake
- Live Worker health endpoint

## Not executable in the creation environment
The environment used to generate this repo does not contain Docker, Flutter, Terraform, kubectl or Helm and has no AWS/Cloudflare/App Store/Google Play credentials. Therefore these commands are provided but were not falsely marked as executed:

- `make local-up`
- `make flutter-get && make flutter-test`
- `make terraform-validate`
- `make helm-lint`
- real AWS/Kubernetes/Cloudflare deployment
- iOS/Android signing and store deployment

Run `make doctor` on the target Mac after `scripts/bootstrap-macos.sh` to confirm prerequisites.

## Important scope
This is a production-oriented **boilerplate/foundation**, not a completed hosted platform. Infrastructure components, contracts, module boundaries and deploy paths are wired and documented. Domain implementations such as full Keycloak login flows, OpenFGA policies, PostgreSQL repositories, Kafka producers/consumers, billing providers and notification providers are intentionally extension points for the platform modules rather than fake implementations.
