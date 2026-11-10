#!/usr/bin/env python3
"""Build the static client release using the runtime's canonical design assets."""
from pathlib import Path
import argparse
import zipfile

root = Path(__file__).resolve().parents[2]
parser = argparse.ArgumentParser()
parser.add_argument('output', type=Path)
args = parser.parse_args()
args.output.parent.mkdir(parents=True, exist_ok=True)
with zipfile.ZipFile(args.output, 'w', zipfile.ZIP_DEFLATED) as archive:
    for path in sorted((root / 'client/web/static').iterdir()):
        archive.write(path, path.name)
    for name in ['mu.css', 'icon-192.png', 'icon-512.png']:
        archive.write(root / 'internal/app/html' / name, name)
    archive.write(root / 'client/web/README.md', 'README.md')
