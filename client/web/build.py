#!/usr/bin/env python3
"""Package the already-built, self-contained client and its Node launcher."""
from pathlib import Path
import argparse
import zipfile

root = Path(__file__).resolve().parent
parser = argparse.ArgumentParser()
parser.add_argument('output', type=Path)
args = parser.parse_args()
args.output.parent.mkdir(parents=True, exist_ok=True)
with zipfile.ZipFile(args.output, 'w', zipfile.ZIP_DEFLATED) as archive:
    for path in sorted((root / 'static').rglob('*')):
        if path.is_file():
            archive.write(path, path.relative_to(root))
    for name in ['serve.mjs', 'README.md']:
        archive.write(root / name, name)
