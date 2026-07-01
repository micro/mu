#!/bin/sh
# Idempotently switch Mu to systemd socket activation for zero-downtime restarts.
#
# Safe by design, meant to run unattended from the deploy pipeline:
#   - never overwrites the existing mu.service (adds a drop-in + a new socket)
#   - a no-op once installed, or if passwordless sudo isn't available
#   - verifies the service is healthy after the one-time cutover and rolls the
#     whole change back if it isn't, leaving you on plain startup
#
# Keep ADDR in sync with nginx's upstream and Mu's --address (default :8080).
set -u

UNIT_DIR=/etc/systemd/system
ADDR="127.0.0.1:8080"

if [ -f "$UNIT_DIR/mu.socket" ]; then
  echo "zero-downtime: socket activation already installed"
  exit 0
fi

if ! sudo -n true 2>/dev/null; then
  echo "zero-downtime: skipped (passwordless sudo not available)"
  exit 0
fi

echo "zero-downtime: installing mu.socket + drop-in (one-time)..."

sudo -n tee "$UNIT_DIR/mu.socket" >/dev/null <<EOF
[Unit]
Description=Mu web socket
[Socket]
ListenStream=$ADDR
Service=mu.service
[Install]
WantedBy=sockets.target
EOF

sudo -n mkdir -p "$UNIT_DIR/mu.service.d"
sudo -n tee "$UNIT_DIR/mu.service.d/10-socket.conf" >/dev/null <<EOF
[Unit]
Requires=mu.socket
After=mu.socket
EOF

rollback() {
  echo "zero-downtime: rolling back to plain startup"
  sudo -n rm -f "$UNIT_DIR/mu.socket" "$UNIT_DIR/mu.service.d/10-socket.conf"
  sudo -n rmdir "$UNIT_DIR/mu.service.d" 2>/dev/null || true
  sudo -n systemctl disable mu.socket 2>/dev/null || true
  sudo -n systemctl daemon-reload
  sudo -n systemctl restart mu.service || true
}

sudo -n systemctl daemon-reload

# Cutover: free :8080 from the running service, hand it to the socket, then let
# the service re-adopt it via the passed fd.
sudo -n systemctl stop mu.service 2>/dev/null || true
if ! sudo -n systemctl start mu.socket; then
  echo "zero-downtime: mu.socket failed to start"
  rollback
  exit 0
fi
sudo -n systemctl enable mu.socket 2>/dev/null || true
sudo -n systemctl start mu.service || true

# Verify both units are active; otherwise undo everything.
sleep 2
if [ "$(sudo -n systemctl is-active mu.socket 2>/dev/null)" = "active" ] \
   && [ "$(sudo -n systemctl is-active mu.service 2>/dev/null)" = "active" ]; then
  echo "zero-downtime: active — redeploys are now seamless"
else
  echo "zero-downtime: post-cutover health check failed"
  rollback
fi
