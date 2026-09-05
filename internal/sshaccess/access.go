package sshaccess

// Registering the key you connect with.
//
// Shown on both /shell and /files because a key proves account identity. What
// it authorises is decided by the SSH request after authentication.
//
// Parsing lives here rather than in internal/auth, deliberately. Auth stores a
// key and its fingerprint and knows nothing about SSH wire format — see
// sshkey.go — so the one place that understands what an authorized_keys line
// is, is the package that also runs the server.

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"golang.org/x/crypto/ssh"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/origin"
	"mu/internal/settings"
)

// Add registers what somebody pasted, and says what happened.
func Add(accountID, line, name string) string {
	print, err := Register(accountID, line, name)
	if err != nil {
		return app.Problem(err.Error())
	}
	return app.Note("Added " + html.EscapeString(print) + ".")
}

// Register adds a public key and returns its fingerprint.
func Register(accountID, line, name string) (string, error) {
	// Parsed before it is stored, so what is kept is a key rather than a
	// string somebody typed. ParseAuthorizedKey is the same function sshd
	// uses on the same file format, which means anything it rejects would
	// never have worked anyway.
	key, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(line)))
	if err != nil {
		return "", fmt.Errorf("that does not look like a public key; paste the contents of a .pub file — the line starting ssh-ed25519 or ssh-rsa")
	}

	// A private key pasted by mistake never reaches ParseAuthorizedKey — it
	// fails above — so the only thing to guard here is the honest mistake of
	// naming it after the comment, which is usually somebody's email address.
	if strings.TrimSpace(name) == "" {
		name = comment
	}
	if strings.TrimSpace(name) == "" {
		name = "key"
	}

	print := ssh.FingerprintSHA256(key)
	stored := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
	if err := auth.AddSSHKey(accountID, name, stored, print); err != nil {
		return "", err
	}
	return print, nil
}

// Card is the account's shared SSH keys and onboarding for one SSH carrier.
func Card(r *http.Request, accountID, action, heading, description, command string) string {
	var b strings.Builder
	b.WriteString(`<div class="card mt-4"><h3>` + html.EscapeString(heading) + `</h3>`)

	port := Port()
	if port == "" || strings.EqualFold(port, "off") {
		// Said rather than hidden. A form that registers a key for a server
		// nobody is running collects credentials for nothing, and the person
		// filling it in has no way to find that out.
		b.WriteString(app.Note("This instance is not running the SSH door, so " +
			"there is nothing to connect to yet. An operator turns it on by " +
			"setting SHELL_SSH_PORT."))
		b.WriteString(`</div>`)
		return b.String()
	}

	b.WriteString(`<p class="text-sm">` + html.EscapeString(description) + `</p>`)
	b.WriteString(`<pre class="raw-sm">` + html.EscapeString(connectLine(command, port)) + `</pre>`)
	b.WriteString(app.Note("Any username works: which key signs the connection " +
		"is what says who you are."))

	if keys := auth.SSHKeys(accountID); len(keys) > 0 {
		b.WriteString(`<table class="data-table mt-3">`)
		b.WriteString(`<tr><th>Name</th><th>Fingerprint</th><th>Last used</th><th></th></tr>`)
		for _, k := range keys {
			used := "never"
			if !k.Used.IsZero() {
				used = app.TimeAgo(k.Used)
			}
			b.WriteString(`<tr><td>` + html.EscapeString(k.Name) + `</td>` +
				`<td class="addr">` + html.EscapeString(k.Print) + `</td>` +
				`<td>` + html.EscapeString(used) + `</td><td>` +
				`<form method="post" action="` + html.EscapeString(action) + `" class="d-inline">` +
				`<input type="hidden" name="csrf_token" value="` +
				html.EscapeString(auth.CSRFToken(r)) + `">` +
				`<input type="hidden" name="removekey" value="` +
				html.EscapeString(k.Print) + `">` +
				`<button type="submit" class="mini-btn danger">Remove</button>` +
				`</form></td></tr>`)
		}
		b.WriteString(`</table>`)
	}

	b.WriteString(`<form method="post" action="` + html.EscapeString(action) + `" class="mt-3">`)
	b.WriteString(`<input type="hidden" name="csrf_token" value="` +
		html.EscapeString(auth.CSRFToken(r)) + `">`)
	b.WriteString(`<input class="form-input w-full" type="text" name="sshkey" ` +
		`placeholder="ssh-ed25519 AAAA… you@laptop" autocomplete="off" spellcheck="false">`)
	b.WriteString(`<input class="form-input mt-2" type="text" name="keyname" ` +
		`placeholder="what to call it (optional)" autocomplete="off">`)
	b.WriteString(`<button type="submit" class="mt-2">Add key</button>`)
	b.WriteString(`</form>`)

	b.WriteString(`</div>`)
	return b.String()
}

// connectLine is the command to copy, with this instance's own host and port
// in it rather than a placeholder somebody has to work out.
func connectLine(command, port string) string {
	host := strings.TrimPrefix(strings.TrimPrefix(origin.Self(), "https://"), "http://")
	host = strings.TrimSuffix(host, "/")
	if host == "" {
		host = "this-instance"
	}
	// The port as configured may be ":2222" or "0.0.0.0:2222"; what somebody
	// types is the number.
	if i := strings.LastIndex(port, ":"); i >= 0 {
		port = port[i+1:]
	}
	if port == "22" {
		return command + " you@" + host
	}
	flag := "-p"
	if command == "sftp" {
		flag = "-P"
	}
	return command + " " + flag + " " + port + " you@" + host
}

// Port is the shared SSH listener setting. The old Sandbox name remains an
// unadvertised migration fallback for instances configured before the rename.
func Port() string {
	if v := strings.TrimSpace(settings.Get("SHELL_SSH_PORT")); v != "" {
		return v
	}
	return strings.TrimSpace(settings.Get(strings.Join([]string{"SANDBOX", "SSH", "PORT"}, "_")))
}
