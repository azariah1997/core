#!/usr/bin/env python3
"""Embeds contracts/openapi/core-api.yaml and contracts/asyncapi/events.yaml
directly into their catalog-info.yaml's spec.definition field.

Why: Backstage's $text placeholder (the natural way to reference a sibling
file) needs the referencing location's baseUrl to be a real URL with a
scheme; a `type: file` catalog location's baseUrl is a bare filesystem
path, so `$text: ./core-api.yaml` throws "could not form a URL" at
ingest time - a real limitation of local file locations in this
Backstage version (0.17.x era), confirmed live (see VALIDATION.md's
Phase 26 entry), not a path or directory-traversal mistake.

Run this after editing either spec file, then re-run
`python3 scripts/validate_contracts.py` and restart Backstage.
"""
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

JOBS = [
    (ROOT / "contracts/openapi/core-api.yaml", ROOT / "contracts/openapi/catalog-info.yaml", "core-api"),
    (ROOT / "contracts/asyncapi/events.yaml", ROOT / "contracts/asyncapi/catalog-info.yaml", "core-platform-events"),
]


def indent(text, spaces):
    pad = " " * spaces
    return "\n".join(pad + line if line else line for line in text.splitlines())


for spec_path, catalog_path, name in JOBS:
    spec_text = spec_path.read_text()
    catalog_text = catalog_path.read_text()
    lines = catalog_text.splitlines()
    out = []
    skipping = False
    for line in lines:
        if line.strip().startswith("definition:"):
            out.append("  definition: |")
            out.append(indent(spec_text.rstrip("\n"), 4))
            skipping = True
            continue
        if skipping:
            # skip the old `$text: ./x.yaml` line, or a prior embedded
            # block's lines (which may include blank lines - a bare ""
            # doesn't start with the 4-space pad, so it needs its own check)
            if line == "" or line.startswith("    ") or line.strip().startswith("$text:"):
                continue
            skipping = False
        out.append(line)
    catalog_path.write_text("\n".join(out) + "\n")
    print(f"embedded {spec_path.relative_to(ROOT)} into {catalog_path.relative_to(ROOT)} ({name})")
