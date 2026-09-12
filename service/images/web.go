package images

import (
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/imagesearch"
	"mu/internal/quota"
	"net/http"
)

func webSearchPage(w http.ResponseWriter, r *http.Request) {
	owner, ok := app.BillableCaller(w, r, quota.OpWebSearch)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "Invalid CSRF token")
		return
	}
	if owner != "" {
		if err := auth.CheckPostRate(owner); err != nil {
			app.RespondError(w, 429, "Please wait before another search.")
			return
		}
	}

	results, err := imagesearch.Search(r.Context(), r.PostFormValue("query"))
	if err == nil && owner != "" {
		err = quota.Charge(owner, quota.OpWebSearch, nil)
	}
	b := `<p><a href="/images">← Images</a></p>`
	if err != nil {
		b += `<p role="alert">` + html.EscapeString(err.Error()) + `</p>`
	} else {
		b += `<div class="thumb-grid">`
		for _, x := range results {
			b += `<figure class="m-0"><a href="` + html.EscapeString(x.URL) + `" target="_blank" rel="noopener noreferrer"><img class="w-full rounded-lg" src="` + html.EscapeString(x.Thumbnail.Src) + `" alt="` + html.EscapeString(x.Title) + `" loading="lazy" referrerpolicy="no-referrer"></a><figcaption class="text-sm">` + html.EscapeString(x.Title) + `<div class="text-muted">` + html.EscapeString(x.Source) + `</div></figcaption></figure>`
		}
		b += `</div>`
		if len(results) == 0 {
			b += `<p>No matching images.</p>`
		}
	}
	app.Respond(w, r, app.Response{Title: "Images", HTML: b})
}
