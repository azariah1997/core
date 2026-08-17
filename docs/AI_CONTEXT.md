# AI Context Contract

Read this file before modifying the repository.

## Non-negotiable rules
- Core modules never import product-specific code.
- Do not add a generic SQL-over-HTTP API.
- Do not bypass authorization at service boundaries.
- Do not add a new infrastructure product when an existing platform capability covers the requirement.
- Any persistent schema change requires a migration.
- Any public API change updates OpenAPI.
- Any event change updates AsyncAPI and increments the event version if incompatible.
- Any cross-service RPC change updates Protobuf.
- Add/adjust tests with behavior changes.
- Update relevant ADR when changing an accepted architecture decision.
- Never commit credentials, signing keys, tokens or personal data.

## Change checklist
1. Identify owning module.
2. Review module.yaml dependencies.
3. Change contract first when applicable.
4. Implement.
5. Test.
6. Validate configs/contracts.
7. Update docs/catalog.
8. Submit PR; never deploy production directly.
