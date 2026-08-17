from pathlib import Path
import sys
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
print('Contract/static deployment validation passed:', ', '.join(checks))
