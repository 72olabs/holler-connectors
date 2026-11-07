#!/usr/bin/env python3
"""Fail if connector builds acquire private core or database dependencies."""
import json
from pathlib import Path
import subprocess
root = Path(__file__).resolve().parents[1]
modules = subprocess.check_output(['go', 'list', '-m', 'all'], cwd=root, text=True)
for forbidden in ('github.com/72olabs/holler ', 'modernc.org/sqlite', 'github.com/mattn/go-sqlite3'):
    if forbidden in modules:
        raise SystemExit('Forbidden dependency: '+forbidden)
for path in root.rglob('*.go'):
    if '.git' in path.parts:
        continue
    source = path.read_text()
    if '"github.com/72olabs/holler/' in source:
        raise SystemExit('Private core import: '+str(path))
for folder in ('internal/store', 'internal/daemon', 'internal/gateway', 'internal/agentruntime'):
    if (root/folder).exists():
        raise SystemExit('Private implementation directory: '+folder)
print('Standalone dependency boundary verified')
