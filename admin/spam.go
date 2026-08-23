package admin

// Refusing mail: what is filtered, and who is barred outright.
//
// Two nav entries and two pages, and one subject. The spam filter scores a
// message and the blocklist refuses a sender before it is scored — different
// mechanisms, same errand, and the operator arrives at both from the same
// event: something got in, or something did not, and it should have been the
// other way round. Tuning the threshold on one page and then going to find the
// other one to block the domain the message came from is two pages for one
// decision.
//
// One page now, and the order on it is the order the mail meets them: the
// filter first, the outright refusals below.
//
// The mail log is not here. It is what happened rather than what the rules are,
// so it is a tab under Logs beside the system log and the external calls — see
// admin/log.go.

import (
	"fmt"
	"html"
	"net/http"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/service/mail"
)

// SpamHandler serves /admin/spam: the filter, and the blocklist under it.
//
// Both sets of actions post here. They do not collide — the filter's are
// toggle, set_threshold and add_/remove_ for the three lists; the blocklist's
// are block_/unblock_ for an address or an IP — and one page that saves in two
// places is still one page.
func SpamHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			app.BadRequest(w, r, "Failed to parse form")
			return
		}
		switch r.FormValue("action") {
		case "block_email", "block_ip", "unblock_email", "unblock_ip":
			if !blocked(w, r) {
				return
			}
		default:
			filtered(r)
		}
		http.Redirect(w, r, "/admin/spam", http.StatusSeeOther)
		return
	}

	content := back() + spamSettings() + blocklistSection()

	app.Respond(w, r, app.Response{Title: "Spam", Description: "What is filtered, and who is refused outright", HTML: content})
}

// BlocklistMoved sends the old address to the page that absorbed it.
func BlocklistMoved(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/spam", http.StatusSeeOther)
}

// blocked applies a blocklist action, and reports whether the page may go on
// to redirect. It writes its own error when it cannot.
func blocked(w http.ResponseWriter, r *http.Request) bool {
	switch r.FormValue("action") {
	case "block_email":
		email := r.FormValue("email")
		if email == "" {
			app.BadRequest(w, r, "Email required")
			return false
		}
		if err := mail.BlockEmail(email); err != nil {
			app.BadRequest(w, r, "Failed to block email: "+err.Error())
			return false
		}
	case "block_ip":
		ip := r.FormValue("ip")
		if ip == "" {
			app.BadRequest(w, r, "IP required")
			return false
		}
		if err := mail.BlockIP(ip); err != nil {
			app.BadRequest(w, r, "Failed to block IP: "+err.Error())
			return false
		}
	case "unblock_email":
		if err := mail.UnblockEmail(r.FormValue("email")); err != nil {
			app.BadRequest(w, r, "Failed to unblock email: "+err.Error())
			return false
		}
	case "unblock_ip":
		if err := mail.UnblockIP(r.FormValue("ip")); err != nil {
			app.BadRequest(w, r, "Failed to unblock IP: "+err.Error())
			return false
		}
	}
	return true
}

// blocklistSection is who is refused outright, below the filter.
//
// Below rather than above, because that is the order a message meets them in
// reverse: the filter is the everyday control and this is the one you reach
// for when the filter was not enough.
func blocklistSection() string {
	bl := mail.GetBlocklist()

	content := `<div class="blocklist-section mt-6">
		<h3>Blocked senders (` + fmt.Sprintf("%d", len(bl.Emails)) + `)</h3>
		<p class="text-sm text-muted">Refused at the door, before anything is scored.</p>
		<div class="block-form">
			<form method="POST">
				<input type="hidden" name="action" value="block_email">
				<input type="text" name="email" placeholder="email@example.com or *@domain.com" required>
				<button type="submit">Block</button>
			</form>
			<p class="text-sm text-muted mt-1">Use *@domain.com to block an entire domain</p>
		</div>`

	if len(bl.Emails) > 0 {
		content += `<table class="blacklist-table"><tbody>`
		for _, email := range bl.Emails {
			content += blocklistEmailRow(email)
		}
		content += `</tbody></table>`
	} else {
		content += `<p class="text-muted">Nobody is blocked.</p>`
	}

	content += `</div>

	<div class="blocklist-section mt-4">
		<h3>Blocked IPs (` + fmt.Sprintf("%d", len(bl.IPs)) + `)</h3>
		<div class="block-form">
			<form method="POST">
				<input type="hidden" name="action" value="block_ip">
				<input type="text" name="ip" placeholder="192.168.1.1" required>
				<button type="submit">Block</button>
			</form>
		</div>`

	if len(bl.IPs) > 0 {
		content += `<table class="blacklist-table"><tbody>`
		for _, ip := range bl.IPs {
			content += blocklistIPRow(ip)
		}
		content += `</tbody></table>`
	} else {
		content += `<p class="text-muted">No blocked IPs.</p>`
	}

	return content + `</div>`
}

func blocklistEmailRow(email string) string {
	escapedEmail := html.EscapeString(email)
	return `
				<tr>
					<td><code>` + escapedEmail + `</code></td>
					<td class="text-center">
						<form method="POST" class="d-inline">
							<input type="hidden" name="action" value="unblock_email">
							<input type="hidden" name="email" value="` + escapedEmail + `">
							<button type="submit" class="btn-success">Unblock</button>
						</form>
					</td>
				</tr>`
}

func blocklistIPRow(ip string) string {
	escapedIP := html.EscapeString(ip)
	return `
				<tr>
					<td><code>` + escapedIP + `</code></td>
					<td class="text-center">
						<form method="POST" class="d-inline">
							<input type="hidden" name="action" value="unblock_ip">
							<input type="hidden" name="ip" value="` + escapedIP + `">
							<button type="submit" class="btn-success">Unblock</button>
						</form>
					</td>
				</tr>`
}

// filtered applies a spam-filter setting. Every one of them is a toggle or a
// list edit that cannot fail in a way worth reporting — the mail package
// stores them and says so in its own log.
func filtered(r *http.Request) {
	value := r.FormValue("value")
	switch r.FormValue("action") {
	case "toggle":
		sf := mail.GetSpamFilter()
		mail.SetSpamFilterEnabled(!sf.Enabled) //nolint:errcheck
	case "set_threshold":
		t := 5
		fmt.Sscanf(value, "%d", &t)
		mail.SetSpamThreshold(t) //nolint:errcheck
	case "toggle_reject":
		sf := mail.GetSpamFilter()
		mail.SetRejectSpam(!sf.RejectSpam) //nolint:errcheck
	case "toggle_autoblock":
		sf := mail.GetSpamFilter()
		mail.SetAutoBlockDomains(!sf.AutoBlockDomains) //nolint:errcheck
	case "add_tld":
		if value != "" {
			mail.AddBlockedTLD(value) //nolint:errcheck
		}
	case "remove_tld":
		if value != "" {
			mail.RemoveBlockedTLD(value) //nolint:errcheck
		}
	case "add_keyword":
		if value != "" {
			mail.AddBlockedKeyword(value) //nolint:errcheck
		}
	case "remove_keyword":
		if value != "" {
			mail.RemoveBlockedKeyword(value) //nolint:errcheck
		}
	case "add_allowed":
		if value != "" {
			mail.AddAllowedSender(value) //nolint:errcheck
		}
	case "remove_allowed":
		if value != "" {
			mail.RemoveAllowedSender(value) //nolint:errcheck
		}
	}
}

// spamSettings is the filter: what it does, and the three lists it reads.
func spamSettings() string {
	sf := mail.GetSpamFilter()

	enabledStatus := "Disabled"
	enabledBtn := "Enable"
	if sf.Enabled {
		enabledStatus = "Enabled"
		enabledBtn = "Disable"
	}

	rejectStatus := "Drop silently"
	rejectBtn := "Switch to reject"
	if sf.RejectSpam {
		rejectStatus = "Save to filtered folder"
		rejectBtn = "Switch to silent drop"
	}

	autoBlockStatus := "Off"
	autoBlockBtn := "Enable"
	if sf.AutoBlockDomains {
		autoBlockStatus = "On"
		autoBlockBtn = "Disable"
	}

	content := fmt.Sprintf(`
	<div class="spam-settings">
		<h3>Settings</h3>
		<table class="blacklist-table">
			<tr>
				<td><strong>Filter Status</strong></td>
				<td>%s</td>
				<td>
					<form method="POST" class="d-inline">
						<input type="hidden" name="action" value="toggle">
						<button type="submit">%s</button>
					</form>
				</td>
			</tr>
			<tr>
				<td><strong>Spam Handling</strong></td>
				<td>%s</td>
				<td>
					<form method="POST" class="d-inline">
						<input type="hidden" name="action" value="toggle_reject">
						<button type="submit">%s</button>
					</form>
				</td>
			</tr>
			<tr>
				<td><strong>Auto-block spam domains</strong></td>
				<td>%s</td>
				<td>
					<form method="POST" class="d-inline">
						<input type="hidden" name="action" value="toggle_autoblock">
						<button type="submit">%s</button>
					</form>
				</td>
			</tr>
			<tr>
				<td><strong>Score Threshold</strong></td>
				<td>%d</td>
				<td>
					<form method="POST" class="d-inline">
						<input type="hidden" name="action" value="set_threshold">
						<input type="number" name="value" value="%d" min="1" max="100" class="w-60">
						<button type="submit">Set</button>
					</form>
				</td>
			</tr>
		</table>
	</div>`, enabledStatus, enabledBtn, rejectStatus, rejectBtn,
		autoBlockStatus, autoBlockBtn, sf.Threshold, sf.Threshold)

	// Blocked TLDs
	content += `<div class="spam-section mt-4">
		<h3>Blocked TLDs (` + fmt.Sprintf("%d", len(sf.BlockedTLDs)) + `)</h3>
		<form method="POST" class="block-form">
			<input type="hidden" name="action" value="add_tld">
			<input type="text" name="value" placeholder=".vn, .xyz, .top" required>
			<button type="submit">Block TLD</button>
		</form>`

	if len(sf.BlockedTLDs) > 0 {
		content += `<table class="blacklist-table"><tbody>`
		for _, tld := range sf.BlockedTLDs {
			content += fmt.Sprintf(`<tr><td><code>%s</code></td><td class="text-center">
				<form method="POST" class="d-inline">
					<input type="hidden" name="action" value="remove_tld">
					<input type="hidden" name="value" value="%s">
					<button type="submit" class="btn-success">Remove</button>
				</form></td></tr>`, tld, tld)
		}
		content += `</tbody></table>`
	}
	content += `</div>`

	// Blocked keywords
	content += `<div class="spam-section mt-4">
		<h3>Blocked Keywords (` + fmt.Sprintf("%d", len(sf.BlockedKeywords)) + `)</h3>
		<form method="POST" class="block-form">
			<input type="hidden" name="action" value="add_keyword">
			<input type="text" name="value" placeholder="keyword or phrase" required>
			<button type="submit">Block Keyword</button>
		</form>`

	if len(sf.BlockedKeywords) > 0 {
		content += `<table class="blacklist-table"><tbody>`
		for _, kw := range sf.BlockedKeywords {
			content += fmt.Sprintf(`<tr><td><code>%s</code></td><td class="text-center">
				<form method="POST" class="d-inline">
					<input type="hidden" name="action" value="remove_keyword">
					<input type="hidden" name="value" value="%s">
					<button type="submit" class="btn-success">Remove</button>
				</form></td></tr>`, kw, kw)
		}
		content += `</tbody></table>`
	}
	content += `</div>`

	// Allowed senders
	content += `<div class="spam-section mt-4">
		<h3>Allowed Senders (` + fmt.Sprintf("%d", len(sf.AllowedSenders)) + `)</h3>
		<p class="text-sm text-muted">These senders bypass spam checks. Use @domain.com for entire domains.</p>
		<form method="POST" class="block-form">
			<input type="hidden" name="action" value="add_allowed">
			<input type="text" name="value" placeholder="user@example.com or @domain.com" required>
			<button type="submit">Allow Sender</button>
		</form>`

	if len(sf.AllowedSenders) > 0 {
		content += `<table class="blacklist-table"><tbody>`
		for _, s := range sf.AllowedSenders {
			content += fmt.Sprintf(`<tr><td><code>%s</code></td><td class="text-center">
				<form method="POST" class="d-inline">
					<input type="hidden" name="action" value="remove_allowed">
					<input type="hidden" name="value" value="%s">
					<button type="submit" class="btn-success">Remove</button>
				</form></td></tr>`, s, s)
		}
		content += `</tbody></table>`
	}
	content += `</div>`

	return content
}
