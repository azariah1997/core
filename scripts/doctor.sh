#!/usr/bin/env bash
set -u
for cmd in git go node npm docker flutter terraform kubectl helm python3; do
  if command -v "$cmd" >/dev/null 2>&1; then printf "%-12s OK  " "$cmd"; "$cmd" --version 2>/dev/null | head -n1 || true
  else printf "%-12s MISSING\n" "$cmd"; fi
done
