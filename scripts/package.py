#!/usr/bin/env python3
"""Build a local connector archive. Does not publish or alter installations."""
import argparse
import hashlib
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile
root = Path(__file__).resolve().parents[1]
p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--version', required=True)
a = p.parse_args()
if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9.+_-]*', a.version):
    p.error('invalid version')
commit = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root, text=True).strip()
dirty = bool(subprocess.check_output(['git', 'status', '--porcelain'], cwd=root, text=True).strip())
goos = subprocess.check_output(['go', 'env', 'GOOS'], text=True).strip()
arch = subprocess.check_output(['go', 'env', 'GOARCH'], text=True).strip()
name = f'holler-connectors-{a.version}-{goos}-{arch}'
dist = root/'dist'
dist.mkdir(exist_ok=True)
with tempfile.TemporaryDirectory(prefix='hc-package-') as tmp:
    stage = Path(tmp)/name
    (stage/'bin').mkdir(parents=True)
    flags = f'-X github.com/72olabs/holler-connectors/buildinfo.Version={a.version} -X github.com/72olabs/holler-connectors/buildinfo.Commit={commit} -X github.com/72olabs/holler-connectors/buildinfo.Dirty={str(dirty).lower()}'
    subprocess.run(['go', 'build', '-trimpath', '-ldflags', flags, '-o', str(stage/'bin/holler-connector'), './cmd/holler-connector'], cwd=root, check=True)
    shutil.copytree(root/'connectors/marketplace', stage/'share/holler/marketplace')
    shutil.copytree(root/'docs', stage/'docs')
    shutil.copytree(root/'third_party', stage/'third_party')
    for file in ('LICENSE', 'NOTICE', 'README.md', 'CONTRIBUTING.md', 'SECURITY.md'):
        shutil.copyfile(root/file, stage/file)
    archive = dist/(name+'.tar.gz')
    with tarfile.open(archive, 'w:gz') as tar:
        tar.add(stage, arcname=name)
    (dist/(archive.name+'.sha256')).write_text(hashlib.sha256(archive.read_bytes()).hexdigest()+'  '+archive.name+'\n')
    print(archive)
