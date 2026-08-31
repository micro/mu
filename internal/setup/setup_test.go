package setup

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/settings"
)

func TestApplyProvider_OpenRouter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OPENROUTER_API_KEY", "")
	prev := settings.Get("OPENROUTER_API_KEY")
	t.Cleanup(func() { settings.Set("OPENROUTER_API_KEY", prev) })
	settings.Set("OPENROUTER_API_KEY", "")

	if err := ApplyProvider("openrouter", "", ""); err == nil {
		t.Fatal("empty key should fail")
	}
	if err := ApplyProvider("openrouter", "sk-or-test", ""); err != nil {
		t.Fatalf("ApplyProvider: %v", err)
	}
	if got := settings.Get("OPENROUTER_API_KEY"); got != "sk-or-test" {
		t.Fatalf("OPENROUTER_API_KEY = %q", got)
	}
}

func TestApplyProvider_Unknown(t *testing.T) {
	if err := ApplyProvider("nope", "k", ""); err == nil {
		t.Fatal("unknown provider should fail")
	}
}

// The admin account `mu setup` creates has a password its owner chose.
//
// It is created by typing one into the form, and it was recorded without the
// flag that says so — so /account told the first admin of every new instance
// that their account "signs in with Google or a passkey and has no password you
// could type", about the one account on the instance that certainly does have
// one.
//
// Drives applySetup rather than Handler: Handler turns away when an admin
// already exists, and auth loads the real accounts.json at package
// initialisation, before any TestMain here could redirect HOME. Everything
// under test is in applySetup; the guard above it is not what was wrong.
func TestTheSetupAdminHasAPasswordItsOwnerChose(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-not-a-real-key")

	// Distinctive, because auth came up holding whatever accounts this machine
	// has and a common name would collide with a real one.
	const id = "setup_test_first_admin"

	form := url.Values{
		"username": {id},
		"password": {"a password they typed"},
		"provider": {"claude"},
		"key":      {"sk-ant-not-a-real-key"},
	}
	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	applySetup(rec, req)
	if i := strings.Index(rec.Body.String(), "text-error"); i >= 0 {
		t.Fatalf("setup refused the form: %.200s", rec.Body.String()[i:])
	}

	if _, err := auth.GetAccount(id); err != nil {
		t.Fatalf("setup created no account: %v", err)
	}
	if !auth.HasSecret(id) {
		t.Error("the account created by typing a password is recorded as having none, " +
			"so /account offers to set one and tells them they signed in with Google")
	}
}

// Choosing Ollama finishes with a model, asked of the server.
//
// It wrote an endpoint and a key and no model. That was survivable while the
// model defaulted to gpt-4o-mini — and the default was a model no Ollama has, so
// finishing setup earned a 404 naming something the operator never typed. The
// default is gone, so without this the reward is an instance that says it is not
// configured: truthful, and just as useless.
//
// The server knows what it has. Somebody who has just said "use Ollama" should
// not then be asked for an id that /models would have answered.
func TestChoosingOllamaStoresTheModelTheServerHas(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, k := range []string{"OPENAI_BASE_URL", "OPENAI_API_KEY", "OPENAI_MODEL"} {
		prev := settings.Get(k)
		settings.Set(k, "")
		t.Cleanup(func() { settings.Set(k, prev) })
		t.Setenv(k, "")
	}

	// A server that answers /models the way an OpenAI-compatible one does.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/models") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"llama3.2:latest"}]}`))
	}))
	t.Cleanup(srv.Close)

	if err := ApplyProvider("ollama", "", srv.URL+"/v1"); err != nil {
		t.Fatalf("ApplyProvider: %v", err)
	}
	if got := settings.Get("OPENAI_BASE_URL"); got != srv.URL+"/v1" {
		t.Errorf("OPENAI_BASE_URL = %q", got)
	}
	if got := settings.Get("OPENAI_MODEL"); got != "llama3.2:latest" {
		t.Errorf("OPENAI_MODEL = %q, want the model the server named — setup that "+
			"stores an endpoint and no model leaves the instance unconfigured", got)
	}
}

// And a server that cannot be reached leaves the model unset.
//
// Storing a guess would put a model id on the instance that is not there, and
// the failure moves from setup — where somebody is watching — to the first
// question somebody asks.
func TestOllamaWithNoServerStoresNoModel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, k := range []string{"OPENAI_BASE_URL", "OPENAI_API_KEY", "OPENAI_MODEL"} {
		prev := settings.Get(k)
		settings.Set(k, "")
		t.Cleanup(func() { settings.Set(k, prev) })
		t.Setenv(k, "")
	}

	// Nothing listening: a port this test owns and never binds.
	if err := ApplyProvider("ollama", "", "http://127.0.0.1:1/v1"); err != nil {
		t.Fatalf("ApplyProvider: %v", err)
	}
	if got := settings.Get("OPENAI_MODEL"); got != "" {
		t.Errorf("OPENAI_MODEL = %q with no server there, want nothing stored", got)
	}
}
