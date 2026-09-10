#!/bin/sh
# Install a staged CI binary without compiling on the live server.
set -eu
stage=$(cd "$1" && pwd)
(cd "$stage" && sha256sum -c SHA256SUMS)

# Use the service's actual executable path, including custom install locations.
start=$(systemctl show --property=ExecStart --value mu.service)
binary=$(printf '%s\n' "$start" | sed -n 's/^{ path=\([^ ;]*\) ;.*/\1/p')
case "$binary" in /*/mu) ;; *) echo 'Cannot identify the Mu executable' >&2; exit 1 ;; esac
binary=$(readlink -f "$binary")
test -f "$binary"

# Copy beside the destination before renaming: never write over a running file.
next=$(mktemp "${binary}.next.XXXXXX")
trap 'rm -f "$next"' EXIT HUP INT TERM
install -m 0755 "$stage/mu" "$next"
"$next" version
cp -p "$binary" "${binary}.previous"
mv -f "$next" "$binary"

# Preserve the existing idempotent socket-activation migration.
sh "$stage/enable-zero-downtime.sh" || true
if sudo -n systemctl restart mu.service && systemctl is-active --quiet mu.service; then
  echo 'Deploy complete'
else
  echo 'Restart failed; restoring the previous binary' >&2
  mv -f "${binary}.previous" "$binary"
  sudo -n systemctl restart mu.service
  exit 1
fi
