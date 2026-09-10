# Deploying Mu with zero-downtime restarts

Mu is a single binary. On a redeploy the process is replaced, and if nginx hits
the port while it's down it returns a 502 (and may trip the upstream out). The
fix is **systemd socket activation**: systemd owns the listening socket and
keeps it open across restarts, so the kernel queues incoming connections during
the swap instead of refusing them. nginx sees a brief latency, never a 502.

Only one Mu process runs at a time (systemd stops the old one before starting
the new one), which matters because Mu's store is file-backed — a blue-green /
overlapping-process scheme risks two writers on the same JSON files. Socket
activation gives seamless restarts *without* that overlap.

Mu already supports this: on startup it adopts the systemd-passed socket
(`LISTEN_FDS`) when present, and otherwise binds `--address` itself, so the
same binary works with or without socket activation.

## Install

```bash
# adjust User/paths in mu.service to match your box, then:
sudo cp scripts/deploy/systemd/mu.socket   /etc/systemd/system/mu.socket
sudo cp scripts/deploy/systemd/mu.service  /etc/systemd/system/mu.service
sudo systemctl daemon-reload
sudo systemctl enable --now mu.socket   # opens the socket
sudo systemctl start mu.service         # starts Mu on it
```

Point nginx at the same address the socket listens on (`127.0.0.1:8080`).

## Redeploy

The main-branch CI build retains its Linux binaries as artifacts. After the
same run passes tests (including the race detector), it calls the deployment
workflow. That workflow selects the server's architecture and transfers the
existing artifact; neither the deploy job nor the server recompiles it.

The server verifies the transfer checksum, checks that the binary runs, and
atomically replaces the executable configured in `mu.service`. The previous
binary is kept alongside it as `mu.previous` and restored if restarting fails.
The socket stays up during the restart. Deployment runs are serialized, and
superseded commits are skipped before transfer.

The server needs SSH, standard Linux file utilities and permission for user
`mu` to restart the service. Go and a source checkout are no longer used by
deployment; keep the service's existing working directory and environment file.

## Notes

- Keep `ListenStream` in `mu.socket` in sync with Mu's `--address` (default
  `:8080`).
- `mu.service` is `Type=simple`; the socket unit holds the fd, so there's no
  `sd_notify` dependency. Connections queued during the restart are served as
  soon as the new process finishes booting — keep startup quick to minimize the
  queue wait.
- Graceful shutdown drains in-flight requests (~10s); `TimeoutStopSec=20` gives
  headroom before SIGKILL. Long-lived SSE streams (`/agent`) are closed at the
  drain deadline and reconnect.
