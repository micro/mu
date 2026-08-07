package home

// Your system, at a glance.
//
// The home screen was a magazine: news, markets, blog, video, images, social,
// prayer. Every card is the world's content and not one of them is yours. So
// after signing up for tools for agents you arrived somewhere that showed you
// headlines — pleasant, and no evidence that anything belonged to you or that
// anything was happening.
//
// This is the other half: what you have and what is in flight. Tasks the agent
// is working on, mail that arrived while you were elsewhere, apps you built,
// credits left. Four counts, each a link to the noun behind it.
//
// Zeroes are shown rather than hidden. A new account with nothing in it is
// still a shape — these are the things you have — and a strip that appears only
// once you have something would never be there when it was most useful.

import (
	"fmt"
	"strings"

	"mu/internal/auth"
	"mu/service/apps"
	"mu/service/mail"
	"mu/service/tasks"
	"mu/service/wallet"
)

// systemStrip renders the signed-in viewer's own counts. Empty for a guest,
// who has no system to show.
func systemStrip(acc *auth.Account) string {
	if acc == nil {
		return ""
	}

	open, running := 0, 0
	for _, t := range tasks.List(acc.ID, "") {
		switch {
		case t.Status == tasks.StatusDoing:
			running++
			open++
		case t.Status == tasks.StatusTodo:
			open++
		}
	}

	taskNote := ""
	if running > 0 {
		taskNote = fmt.Sprintf("%d working", running)
	}

	// A count with a one-word label is only legible to somebody who already
	// knows what the noun means here. "Apps" could be apps you use; "Unread"
	// could be anything with a number next to it. Each tile now says whose
	// thing it is and what happens if you open it — a line small enough to
	// ignore once you have learned it, and the only thing on the strip that
	// answers "why am I looking at this".
	var b strings.Builder
	b.WriteString(`<div id="home-system">`)
	stat(&b, "/tasks", "Tasks", open, taskNote, "yours to do, or the agent's")
	stat(&b, "/mail", "Unread", mail.GetUnreadCount(acc.ID), "", "in your inbox here")
	stat(&b, "/apps", "Apps", len(apps.GetAppsByAuthor(acc.ID)), "", "you have built")
	// Admins are never charged, so a number here would be meaningless — the
	// same reason the top bar shows them ∞ rather than a balance.
	if !acc.Admin {
		stat(&b, "/wallet", "Credits", wallet.GetBalance(acc.ID), "", "left to spend on calls")
	}
	b.WriteString(`</div>` + systemCSS)
	return b.String()
}

func stat(b *strings.Builder, href, label string, n int, note, hint string) {
	cls := "home-stat"
	if n == 0 {
		cls += " zero"
	}
	extra := ""
	if note != "" {
		extra = `<span class="home-stat-note">` + note + `</span>`
	}
	if hint != "" {
		extra += `<span class="home-stat-h">` + hint + `</span>`
	}
	fmt.Fprintf(b, `<a class="%s" href="%s"><span class="home-stat-n">%d</span>`+
		`<span class="home-stat-l">%s</span>%s</a>`, cls, href, n, label, extra)
}

const systemCSS = `<style>
#home-system{display:flex;flex-wrap:wrap;gap:10px;margin:0 0 16px}
#home-system a{flex:1 1 140px;min-width:140px;text-decoration:none;color:var(--text-primary);
  border:1px solid var(--border-color,#e5e5e5);border-radius:10px;padding:10px 12px;
  display:flex;flex-direction:column;gap:1px;background:#fff}
#home-system a:hover{border-color:#bbb}
.home-stat-n{font-size:22px;font-weight:600;font-variant-numeric:tabular-nums;line-height:1.15}
.home-stat-l{font-size:12px;color:var(--text-muted)}
.home-stat-note{font-size:11px;color:#a86400;font-weight:600}
.home-stat-h{font-size:11px;color:#aaa;line-height:1.3;margin-top:2px}
#home-system a.zero .home-stat-n{color:var(--text-muted);font-weight:500}
@media only screen and (max-width:600px){
  #home-system a{flex:1 1 calc(50% - 5px);min-width:0}
  .home-stat-n{font-size:19px}
}
</style>`
