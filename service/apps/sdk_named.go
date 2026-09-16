package apps

import (
	"mu/internal/service"
	"net/http"
	"strings"
)

func namedSDK(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	if strings.HasSuffix(r.URL.Path, ".d.ts") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(service.TypeScriptSDK(service.Specs())))
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Write([]byte(service.JavaScriptSDK(service.Specs())))
}

func namedShim() string {
	source := strings.TrimPrefix(service.JavaScriptSDK(service.Specs()), "export ")
	return "<script>(function(){" + source + `
 const named = createClient((service, method, args) => window.mu.service(service, method, args));
 // Existing apps have positional helpers such as mu.blog.read(id) and scoped
 // mu.db storage. Preserve them; the client exposes the uniform RPC contract.
 window.mu.createClient = () => named;
 for (const service of Object.keys(named)) {
   if (!window.mu[service]) window.mu[service] = {};
   for (const method of Object.keys(named[service])) {
     if (!window.mu[service][method]) window.mu[service][method] = named[service][method];
   }
 }
 })();</script>`
}
