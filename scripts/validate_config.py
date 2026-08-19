from pathlib import Path
import re, sys
root=Path(__file__).resolve().parents[1]
required=['README.md','.env.example','go.work','infra/docker/docker-compose.yml','contracts/openapi/core-api.yaml','catalog/system.yaml','docs/AI_CONTEXT.md']
missing=[p for p in required if not (root/p).exists()]
if missing: print('Missing:',missing); sys.exit(1)
env=(root/'.env.example').read_text()
keys=[line.split('=',1)[0] for line in env.splitlines() if line and not line.startswith('#')]
if len(keys)!=len(set(keys)): print('Duplicate env keys'); sys.exit(1)
# Third-party/vendored/build trees are never ours to scan - and by
# Phase 26 (platform/backstage's real node_modules) they're large
# enough that scanning them is both slow and prone to false positives
# matching this regex inside someone else's minified/generated code.
SKIP_DIRS={'node_modules','.git','.yarn','dist','dist-types','build','.cache','coverage'}
for p in root.rglob('*'):
    if SKIP_DIRS & set(p.parts):
        continue
    if p.is_file() and p.stat().st_size<2_000_000:
        txt=p.read_text(errors='ignore')
        if re.search(r'AKIA[0-9A-Z]{16}',txt): print('Possible AWS key in',p); sys.exit(1)
print(f'Config validation passed ({len(required)} required artifacts, {len(keys)} env keys)')
