"""Assign every Go package to one stable race-test shard."""
import sys
import zlib

shard, count = map(int, sys.argv[1:])
if not 0 <= shard < count:
    raise SystemExit("shard must be between zero and count - 1")
for line in sys.stdin:
    package = line.strip()
    if package and zlib.crc32(package.encode()) % count == shard:
        print(package)
