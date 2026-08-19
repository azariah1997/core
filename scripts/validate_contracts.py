from pathlib import Path
import sys
import yaml

root=Path(__file__).resolve().parents[1]
checks={
 'openapi':('contracts/openapi/core-api.yaml','openapi: 3.1.0'),
 'asyncapi':('contracts/asyncapi/events.yaml','asyncapi: 3.0.0'),
 'protobuf':('contracts/proto/platform/v1/events.proto','syntax = "proto3";'),
 'migration':('data/migrations/0001_core.sql','CREATE TABLE IF NOT EXISTS outbox_events'),
 'gitops':('infra/argocd/core-platform.yaml','kind: Application'),
 'helm':('infra/kubernetes/charts/core-platform/Chart.yaml','apiVersion: v2'),
}
for name,(file,needle) in checks.items():
    text=(root/file).read_text()
    if needle not in text: print(f'{name} validation failed'); sys.exit(1)

# Real YAML parsing, not just a substring check - Phase 26 found two files
# in this list that "looked right" but had genuinely unparseable YAML (an
# unquoted flow-mapping description containing a comma), which Backstage's
# own catalog/API loaders would have rejected the same way this catches now.
yaml_files = [
    'contracts/openapi/core-api.yaml',
    'contracts/asyncapi/events.yaml',
    'catalog/system.yaml',
    'platform/backstage/catalog-info.yaml',
    'contracts/openapi/catalog-info.yaml',
    'contracts/asyncapi/catalog-info.yaml',
]
for f in yaml_files:
    try:
        list(yaml.safe_load_all((root/f).read_text()))
    except yaml.YAMLError as e:
        print(f'{f}: invalid YAML - {e}')
        sys.exit(1)

# Every operationId in the OpenAPI spec must be unique - Backstage and any
# real code generator both require this.
openapi = yaml.safe_load((root/'contracts/openapi/core-api.yaml').read_text())
seen_ids = {}
for path, methods in openapi['paths'].items():
    for method, op in methods.items():
        if not isinstance(op, dict) or 'operationId' not in op:
            continue
        opid = op['operationId']
        if opid in seen_ids:
            print(f'openapi: duplicate operationId {opid!r} ({method.upper()} {path} and {seen_ids[opid]})')
            sys.exit(1)
        seen_ids[opid] = f'{method.upper()} {path}'

# Every catalog entity reference (dependsOn/providesApis/consumesApis/
# subcomponentOf/owner/system) must resolve to an entity defined
# somewhere in the real catalog - a dangling reference is exactly what
# Backstage's catalog processor would flag as a broken-relations error
# at ingest time. This must cover every file actually registered as a
# catalog.locations entry in platform/backstage/app-config.yaml, not
# just catalog/system.yaml - the two API entities live colocated with
# their spec files (contracts/openapi, contracts/asyncapi) instead,
# since Backstage's file reader won't resolve a $text ref that
# traverses outside the directory of the location that defined it.
catalog_files = [
    'catalog/system.yaml',
    'platform/backstage/catalog-info.yaml',
    'contracts/openapi/catalog-info.yaml',
    'contracts/asyncapi/catalog-info.yaml',
]
catalog_docs = []
for f in catalog_files:
    catalog_docs += list(yaml.safe_load_all((root/f).read_text()))
names = set()
for d in catalog_docs:
    kind = d['kind'].lower()
    names.add(f"{kind}:{d['metadata']['name']}")
default_kind = {
    'dependsOn': 'component', 'subcomponentOf': 'component',
    'providesApis': 'api', 'consumesApis': 'api',
}
for d in catalog_docs:
    spec = d.get('spec', {})
    refs = []
    for field in ('dependsOn', 'providesApis', 'consumesApis'):
        refs += [(field, v) for v in spec.get(field, [])]
    if spec.get('subcomponentOf'):
        refs.append(('subcomponentOf', spec['subcomponentOf']))
    for field, ref in refs:
        if ':' in ref:
            kind, name = ref.split(':', 1)
        else:
            kind, name = default_kind[field], ref
        if f'{kind}:{name}' not in names:
            print(f"catalog: {d['kind']}/{d['metadata']['name']} {field} -> {ref} does not resolve")
            sys.exit(1)
    for field in ('system', 'owner'):
        val = spec.get(field)
        if not val:
            continue
        implied_kind = 'system' if field == 'system' else 'group'
        if f'{implied_kind}:{val}' not in names and val not in names:
            print(f"catalog: {d['kind']}/{d['metadata']['name']} {field} -> {val} does not resolve")
            sys.exit(1)

print('Contract/static deployment validation passed:', ', '.join(checks), '+ YAML parse + operationId uniqueness + catalog reference checks')
