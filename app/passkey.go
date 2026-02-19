package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"mu/auth"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

var (
	webAuthn        *webauthn.WebAuthn
	webAuthnOnce    sync.Once
	sessionStore    = map[string]*webauthn.SessionData{} // challenge -> session
	sessionStoreMu  sync.Mutex
)

func getWebAuthn(r *http.Request) *webauthn.WebAuthn {
	webAuthnOnce.Do(func() {
		rpID := os.Getenv("PASSKEY_RP_ID")
		if rpID == "" {
			rpID = "localhost"
		}
		origin := os.Getenv("PASSKEY_ORIGIN")
		if origin == "" {
			origin = "http://localhost:8080"
		}

		var err error
		webAuthn, err = webauthn.New(&webauthn.Config{
			RPDisplayName: "Mu",
			RPID:          rpID,
			RPOrigins:     []string{origin},
			AuthenticatorSelection: protocol.AuthenticatorSelection{
				RequireResidentKey: protocol.ResidentKeyRequired(),
				ResidentKey:        protocol.ResidentKeyRequirementRequired,
				UserVerification:   protocol.VerificationPreferred,
			},
		})
		if err != nil {
			Log("auth", "WebAuthn init error: %v", err)
		}
	})
	return webAuthn
}

func storeSession(key string, session *webauthn.SessionData) {
	sessionStoreMu.Lock()
	defer sessionStoreMu.Unlock()

	// Clean up expired sessions (older than 5 minutes)
	now := time.Now()
	for k, s := range sessionStore {
		if now.After(s.Expires) {
			delete(sessionStore, k)
		}
	}

	sessionStore[key] = session
}

func getSession(key string) (*webauthn.SessionData, bool) {
	sessionStoreMu.Lock()
	defer sessionStoreMu.Unlock()

	session, ok := sessionStore[key]
	if ok {
		delete(sessionStore, key)
	}
	return session, ok
}

// PasskeyHandler routes passkey requests
func PasskeyHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/passkey")

	switch {
	case path == "/register/begin" && r.Method == "POST":
		passkeyRegisterBegin(w, r)
	case path == "/register/finish" && r.Method == "POST":
		passkeyRegisterFinish(w, r)
	case path == "/login/begin" && r.Method == "POST":
		passkeyLoginBegin(w, r)
	case path == "/login/finish" && r.Method == "POST":
		passkeyLoginFinish(w, r)
	case path == "/delete" && r.Method == "POST":
		passkeyDelete(w, r)
	default:
		http.NotFound(w, r)
	}
}

func passkeyRegisterBegin(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		RespondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	wan := getWebAuthn(r)
	if wan == nil {
		RespondError(w, http.StatusInternalServerError, "WebAuthn not configured")
		return
	}

	user := auth.NewWebAuthnUser(acc)

	options, session, err := wan.BeginRegistration(user)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to begin registration")
		return
	}

	storeSession(acc.ID+":register", session)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(options)
}

func passkeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		RespondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	wan := getWebAuthn(r)
	if wan == nil {
		RespondError(w, http.StatusInternalServerError, "WebAuthn not configured")
		return
	}

	session, ok := getSession(acc.ID + ":register")
	if !ok {
		RespondError(w, http.StatusBadRequest, "no registration in progress")
		return
	}

	user := auth.NewWebAuthnUser(acc)

	credential, err := wan.FinishRegistration(user, *session, r)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "registration failed: "+err.Error())
		return
	}

	// Save the passkey
	pk := &auth.Passkey{
		ID:         uuid.New().String(),
		Name:       "Passkey",
		Account:    acc.ID,
		Credential: *credential,
		Created:    time.Now(),
	}

	if err := auth.SavePasskey(pk); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to save passkey")
		return
	}

	RespondJSON(w, map[string]interface{}{
		"success": true,
		"id":      pk.ID,
		"name":    pk.Name,
	})
}

func passkeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	wan := getWebAuthn(r)
	if wan == nil {
		RespondError(w, http.StatusInternalServerError, "WebAuthn not configured")
		return
	}

	options, session, err := wan.BeginDiscoverableLogin()
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to begin login")
		return
	}

	storeSession(session.Challenge, session)

	var secure bool
	if h := r.Header.Get("X-Forwarded-Proto"); h == "https" {
		secure = true
	}

	// Set the challenge in a cookie so we can retrieve the session on finish
	http.SetCookie(w, &http.Cookie{
		Name:     "passkey_challenge",
		Value:    session.Challenge,
		Path:     "/",
		MaxAge:   300, // 5 minutes
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(options)
}

func passkeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	wan := getWebAuthn(r)
	if wan == nil {
		RespondError(w, http.StatusInternalServerError, "WebAuthn not configured")
		return
	}

	// Get the challenge from cookie
	cookie, err := r.Cookie("passkey_challenge")
	if err != nil {
		RespondError(w, http.StatusBadRequest, "no login in progress")
		return
	}

	session, ok := getSession(cookie.Value)
	if !ok {
		RespondError(w, http.StatusBadRequest, "login session expired")
		return
	}

	// Clear the challenge cookie
	var secure bool
	if h := r.Header.Get("X-Forwarded-Proto"); h == "https" {
		secure = true
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "passkey_challenge",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	// Handler to look up user by their WebAuthn user handle
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		return auth.FindUserByWebAuthnID(userHandle)
	}

	user, credential, err := wan.FinishPasskeyLogin(handler, *session, r)
	if err != nil {
		RespondError(w, http.StatusUnauthorized, "login failed")
		return
	}

	// Update credential usage
	auth.UpdatePasskeyUsage(credential.ID, credential.Authenticator.SignCount)

	// Create a session for the authenticated user
	accountID := string(user.WebAuthnID())

	sess, err := auth.CreateSession(accountID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sess.Token,
		Path:     "/",
		MaxAge:   2592000,
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	RespondJSON(w, map[string]interface{}{
		"success":  true,
		"redirect": "/home",
	})
}

func passkeyDelete(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		RespondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	r.ParseForm()
	passkeyID := r.FormValue("id")
	if passkeyID == "" {
		RespondError(w, http.StatusBadRequest, "passkey ID required")
		return
	}

	if err := auth.DeletePasskey(passkeyID, acc.ID); err != nil {
		RespondError(w, http.StatusForbidden, err.Error())
		return
	}

	// If JSON request, respond with JSON
	if WantsJSON(r) || SendsJSON(r) {
		RespondJSON(w, map[string]interface{}{"success": true})
		return
	}

	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

// PasskeyListHTML returns HTML for listing passkeys on the account page
func PasskeyListHTML(accountID string) string {
	pks := auth.GetPasskeys(accountID)

	var rows string
	for _, pk := range pks {
		created := pk.Created.Format("Jan 2, 2006")
		lastUsed := "Never"
		if !pk.LastUsed.IsZero() {
			lastUsed = TimeAgo(pk.LastUsed)
		}
		credIDDisplay := base64.RawURLEncoding.EncodeToString(pk.Credential.ID)
		if len(credIDDisplay) > 16 {
			credIDDisplay = credIDDisplay[:16] + "..."
		}
		rows += fmt.Sprintf(`<tr>
<td>%s</td>
<td><code>%s</code></td>
<td>%s</td>
<td>%s</td>
<td><form method="POST" action="/passkey/delete" onsubmit="return confirm('Remove this passkey?')"><input type="hidden" name="id" value="%s"><button type="submit">Remove</button></form></td>
</tr>`, pk.Name, credIDDisplay, created, lastUsed, pk.ID)
	}

	if rows == "" {
		rows = `<tr><td colspan="5" style="padding: 20px; text-align: center; color: #666;">No passkeys registered. Add one below.</td></tr>`
	}

	return fmt.Sprintf(`<div class="card">
<h3>Passkeys</h3>
<p>Sign in without a password using your device's biometrics or security key.</p>
<table>
<thead><tr><th>Name</th><th>Key</th><th>Created</th><th>Last Used</th><th></th></tr></thead>
<tbody>%s</tbody>
</table>
<button onclick="registerPasskey()" style="margin-top: 15px;">Add Passkey</button>
<script>
async function registerPasskey() {
  try {
    const beginRes = await fetch('/passkey/register/begin', {method: 'POST'});
    if (!beginRes.ok) { alert('Failed to start registration'); return; }
    const options = await beginRes.json();

    options.publicKey.challenge = base64urlToBuffer(options.publicKey.challenge);
    options.publicKey.user.id = base64urlToBuffer(options.publicKey.user.id);
    if (options.publicKey.excludeCredentials) {
      options.publicKey.excludeCredentials = options.publicKey.excludeCredentials.map(c => ({
        ...c, id: base64urlToBuffer(c.id)
      }));
    }

    const credential = await navigator.credentials.create(options);

    const attestation = {
      id: credential.id,
      rawId: bufferToBase64url(credential.rawId),
      type: credential.type,
      response: {
        attestationObject: bufferToBase64url(credential.response.attestationObject),
        clientDataJSON: bufferToBase64url(credential.response.clientDataJSON)
      }
    };
    if (credential.response.getTransports) {
      attestation.response.transports = credential.response.getTransports();
    }
    if (credential.authenticatorAttachment) {
      attestation.authenticatorAttachment = credential.authenticatorAttachment;
    }

    const finishRes = await fetch('/passkey/register/finish', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(attestation)
    });
    const result = await finishRes.json();
    if (result.success) {
      location.reload();
    } else {
      alert('Registration failed');
    }
  } catch (e) {
    if (e.name !== 'NotAllowedError') alert('Error: ' + e.message);
  }
}

function base64urlToBuffer(b64) {
  const pad = b64.length %% 4;
  if (pad) b64 += '='.repeat(4 - pad);
  const str = atob(b64.replace(/-/g, '+').replace(/_/g, '/'));
  const buf = new Uint8Array(str.length);
  for (let i = 0; i < str.length; i++) buf[i] = str.charCodeAt(i);
  return buf.buffer;
}

function bufferToBase64url(buf) {
  const bytes = new Uint8Array(buf);
  let str = '';
  for (const b of bytes) str += String.fromCharCode(b);
  return btoa(str).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}
</script>
</div>`, rows)
}
