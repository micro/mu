#!/bin/sh
# Enable systemd socket activation for mu's web service so redeploys don't 502.
#
# systemd owns the listening socket and keeps it open across restarts, so the
# kernel queues connections during the swap instead of refusing them. The port
# is auto-detected from mu.service's --address, so this adapts to your box.
#
# Safe by design: never overwrites mu.service (adds a drop-in + a new socket),
# waits for the port to actually free before binding, verifies both units are
# active afterwards and rolls the whole change back if not. No-op once installed
# or without root / passwordless sudo. Run as root:  bash enable-zero-downtime.sh
set -u

SVC=mu.service
SOCK=mu.socket
UNIT_DIR=/etc/systemd/system

# Run privileged commands directly when root, else via non-interactive sudo.
run() { if [ "$(id -u)" = 0 ]; then "$@"; else sudo -n "$@"; fi; }

port_of()  { printf '%s' "$1" | sed -E 's/.*:([0-9]+)$/\1/'; }
port_busy(){ ss -ltnH 2>/dev/null | awk '{print $4}' | grep -qE "[:.]$1\$"; }

if [ -f "$UNIT_DIR/$SOCK" ]; then
  echo "zero-downtime: already installed"; exit 0
fi
if ! run true 2>/dev/null; then
  echo "zero-downtime: skipped (need root or passwordless sudo)"; exit 0
fi

# Discover the address mu.service actually binds (e.g. ":8081").
EXEC=$(systemctl show -p ExecStart --value "$SVC" 2>/dev/null)
ADDR=$(printf '%s' "$EXEC" | grep -oE '\-\-address=[^ ;]+' | head -1 | cut -d= -f2)
[ -n "$ADDR" ] || ADDR=":8080"
PORT=$(port_of "$ADDR")
case "$ADDR" in
  :*) LISTEN="$PORT" ;;   # ":8081" -> port only (all interfaces)
  *)  LISTEN="$ADDR" ;;   # "host:port" -> verbatim
esac
echo "zero-downtime: $SVC binds $ADDR -> socket ListenStream=$LISTEN (port $PORT)"

echo "zero-downtime: writing $SOCK + drop-in..."
run tee "$UNIT_DIR/$SOCK" >/dev/null <<EOF
[Unit]
Description=Mu web socket
[Socket]
ListenStream=$LISTEN
Service=$SVC
[Install]
WantedBy=sockets.target
EOF
run mkdir -p "$UNIT_DIR/$SVC.d"
run tee "$UNIT_DIR/$SVC.d/10-socket.conf" >/dev/null <<EOF
[Unit]
Requires=$SOCK
After=$SOCK
EOF

rollback() {
  echo "zero-downtime: rolling back to plain startup"
  run rm -f "$UNIT_DIR/$SOCK" "$UNIT_DIR/$SVC.d/10-socket.conf"
  run rmdir "$UNIT_DIR/$SVC.d" 2>/dev/null || true
  run systemctl disable "$SOCK" 2>/dev/null || true
  run systemctl daemon-reload
  run systemctl start "$SVC" || true
}

run systemctl daemon-reload

# Cutover: stop the service and wait until the port is genuinely released.
run systemctl stop "$SVC" || { echo "zero-downtime: could not stop $SVC"; rollback; exit 0; }
i=0
while [ "$i" -lt 40 ]; do
  port_busy "$PORT" || break
  sleep 0.25; i=$((i + 1))
done
if port_busy "$PORT"; then
  echo "zero-downtime: port $PORT still held after stopping $SVC:"
  ss -ltnp 2>/dev/null | grep -E "[:.]$PORT " || true
  rollback; exit 0
fi

run systemctl start "$SOCK" || { echo "zero-downtime: socket failed to bind $LISTEN"; rollback; exit 0; }
run systemctl enable "$SOCK" 2>/dev/null || true
run systemctl start "$SVC" || true

sleep 2
if [ "$(run systemctl is-active "$SOCK" 2>/dev/null)" = active ] \
   && [ "$(run systemctl is-active "$SVC" 2>/dev/null)" = active ]; then
  echo "zero-downtime: active on $LISTEN — redeploys are now seamless"
else
  echo "zero-downtime: post-cutover health check failed"
  rollback
fi
