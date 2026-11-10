package app

import (
	"crypto/sha256"
	"fmt"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func TestShellAssetsUseContentVersions(t *testing.T) {
	for _, name := range []string{"mu.css", "mu.js"} {
		source, err := htmlFiles.ReadFile("html/" + name)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(source)
		expected := "/" + name + "?v=" + fmt.Sprintf("%x", digest[:8])
		for _, page := range []string{ConsoleHTML("Home", "", nil), RenderIndex(Index{Title: "Micro"})} {
			if !strings.Contains(page, expected) {
				t.Fatalf("shell does not version %s by its contents", name)
			}
		}
	}
}

func TestStylesOnlyCacheMatchingContentVersion(t *testing.T) {
	for _, tc := range []struct{ url, cache string }{
		{"/mu.css", "no-cache"},
		{"/mu.css?v=layout-142", "no-cache"},
		{"/mu.css?v=" + stylesVersion, "public, max-age=86400"},
	} {
		w := httptest.NewRecorder()
		serveStyles(w, httptest.NewRequest("GET", tc.url, nil))
		if got := w.Header().Get("Cache-Control"); got != tc.cache {
			t.Errorf("%s: cache=%q", tc.url, got)
		}
		if w.Body.String() != Styles() {
			t.Fatal("unexpected stylesheet")
		}
	}
}

// mu.js is served both as the page script and as the installed PWA worker.
func TestScriptInitializesAsServiceWorker(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable")
	}
	script := `const fs=require('fs'), vm=require('vm'); const events=[];
 vm.runInNewContext(fs.readFileSync('html/mu.js','utf8'), {self:{addEventListener:name=>events.push(name)}});
 for (const name of ['fetch','install','activate','push','notificationclick']) {
  if (!events.includes(name)) throw Error('Missing worker handler: '+name);
 }`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("worker startup: %v\n%s", err, out)
	}
}

func TestWorkerPreservesOtherInstallationCaches(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable")
	}
	script := `const fs=require('fs'), vm=require('vm'); const events={}, removed=[];
 vm.runInNewContext(fs.readFileSync('html/mu.js','utf8'), {
 console:{log(){}},self:{addEventListener:(n,f)=>events[n]=f,clients:{claim:async()=>{}}},
 caches:{keys:async()=>['mu_old','mu_v3-compact','mu-web-v1','another-app'],delete:async k=>removed.push(k)}
 });
 events.activate({waitUntil:p=>p.then(()=>{
 if(JSON.stringify(removed)!==JSON.stringify(['mu_old']))throw Error('Deleted unrelated app caches');
 }).catch(e=>{console.error(e);process.exitCode=1;})});`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("cache isolation: %v\n%s", err, out)
	}
}
