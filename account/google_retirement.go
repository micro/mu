package account

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"mu/internal/data"
)

// Retire the old data grants once on upgrade. Local credentials and their
// shrink-recovery copy are removed even if Google's revocation endpoint is
// unavailable, matching Disconnect's previous behavior. No sign-in state is
// stored here. Requests run off the startup path and never log credentials.
func retireGoogleGrants() error {
	return retireGoogleGrantsWith(&http.Client{Timeout: 5 * time.Second})
}

func retireGoogleGrantsWith(client *http.Client) error {
	tokens := map[string]bool{}
	var keys []string
	for _, key := range []string{"google_connections.json", "google_connections.json.prev"} {
		var entries []struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := data.LoadJSON(key, &entries); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("could not read legacy credentials: %w", err)
		}
		for _, entry := range entries {
			if entry.RefreshToken != "" {
				tokens[entry.RefreshToken] = true
			}
		}
		keys = append(keys, key)
	}
	for _, key := range keys {
		if err := data.DeleteFile(key); err != nil {
			return fmt.Errorf("could not remove legacy credentials: %w", err)
		}
	}
	failed := 0
	for token := range tokens {
		response, err := client.PostForm("https://oauth2.googleapis.com/revoke", url.Values{"token": {token}})
		if err != nil {
			failed++
			continue
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("removed local credentials; %d remote revocations unconfirmed (review Google's account permissions)", failed)
	}
	return nil
}
