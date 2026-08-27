package shell

// Reading this service's settings, under either of its names.
//
// The service was called sandbox and its ten settings were SANDBOX_*. Renaming
// the package renames nothing on a running instance: SHELL_SSH_PORT is not what
// is in an operator's environment or in settings.json, and a build that reads
// only the new name finds nothing there. The SSH door would go quiet, the
// memory and CPU caps would silently fall back to their defaults, and a
// shared-mode instance would come back up unshared — all on a deploy whose
// changelog says "renamed a package".
//
// So both names work, new one first. This is not a permanent shim: it is here
// so the rename can ship without an outage, and it can go once no instance
// anybody cares about is still configured the old way. Until then the old name
// is not deprecated in the sense of "still works but do not" — it is what every
// already-running instance is actually using.

import (
	"strings"

	"mu/internal/settings"
)

// setting reads one of this service's settings, falling back to the name it had
// before the rename.
//
// It takes the whole key rather than the part after the prefix, so the string
// an operator would search for is in the source at the point it is read. The
// short form was neater and made every one of these invisible to
// TestEveryConfigVarIsDocumented, which finds settings by looking for their
// names — so four documented settings read as documented-but-unused.
func setting(key string) string {
	if v := strings.TrimSpace(settings.Get(key)); v != "" {
		return v
	}
	if !strings.HasPrefix(key, prefix) {
		return ""
	}
	return strings.TrimSpace(settings.Get(oldPrefix + strings.TrimPrefix(key, prefix)))
}

const (
	prefix    = "SHELL_"
	oldPrefix = "SANDBOX_"
)
