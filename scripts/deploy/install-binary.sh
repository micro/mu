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
# Type=simple becomes active before initialization finishes. Require the same
# process to remain active for ten seconds, including across automatic restarts.
stable() {
  pid=$(systemctl show --property=MainPID --value mu.service)
  case "$pid" in ''|0|*[!0-9]*) return 1 ;; esac
  for check in 1 2 3 4 5; do
    sleep 2
    systemctl is-active --quiet mu.service || return 1
    test "$(systemctl show --property=MainPID --value mu.service)" = "$pid" || return 1
  done
}
if sudo -n systemctl restart mu && stable; then
  echo 'Deploy complete'
else
  echo 'Restart failed; restoring the previous binary' >&2
  mv -f "${binary}.previous" "$binary"
  sudo -n systemctl restart mu
  exit 1
fi
