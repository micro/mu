package test

// The tool count we advertise has to be the tool count we have.
//
// server.json said 67 while the server answered 113. Nothing was wrong with
// either number when it was written; the catalogue grew and the claim did not,
// and a registry listing is exactly the place a stale number does damage — it
// is read by people deciding whether to connect, and it is the one number they
// can check in one request.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"mu/internal/api"
)

func TestTheAdvertisedToolCountIsTheRealOne(t *testing.T) {
	// Counted from the registry the MCP server derives its own list from, so
	// this cannot drift from what a caller is actually served.
	actual := 0
	for _, spec := range allSpecs() {
		actual += len(spec.Endpoints)
	}
	if n := api.ToolCount(); n > 0 {
		actual = n
	}
	if actual == 0 {
		t.Skip("nothing registered in this test binary")
	}

	claim := regexp.MustCompile(`(\d+) tools`)

	for _, f := range []string{"../server.json", "../docs/LISTING.md"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(f), err)
			continue
		}
		for _, m := range claim.FindAllStringSubmatch(string(b), -1) {
			n, err := strconv.Atoi(m[1])
			if err != nil {
				continue
			}
			if n != actual {
				t.Errorf("%s claims %d tools, the server has %d — update the claim",
					filepath.Base(f), n, actual)
			}
		}
	}
}

func TestServerJSONIsValidForTheRegistry(t *testing.T) {
	b, err := os.ReadFile("../server.json")
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Schema      string `json:"$schema"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Version     string `json:"version"`
		WebsiteURL  string `json:"websiteUrl"`
		Repository  struct {
			URL    string `json:"url"`
			Source string `json:"source"`
		} `json:"repository"`
		Remotes []struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"remotes"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatalf("server.json is not valid JSON: %v", err)
	}

	for _, c := range []struct{ name, got string }{
		{"$schema", s.Schema}, {"name", s.Name}, {"description", s.Description},
		{"version", s.Version}, {"websiteUrl", s.WebsiteURL},
		{"repository.url", s.Repository.URL},
	} {
		if c.got == "" {
			t.Errorf("server.json has no %s, which the registry requires", c.name)
		}
	}

	// A remote server is the whole point of this listing: Mu is an endpoint,
	// not a package somebody installs.
	if len(s.Remotes) == 0 {
		t.Fatal("server.json declares no remotes, so it lists as an unusable server")
	}
	if s.Remotes[0].Type != "streamable-http" {
		t.Errorf("transport %q, want streamable-http", s.Remotes[0].Type)
	}
	if s.Remotes[0].URL == "" {
		t.Error("the remote has no url")
	}

	// The description is what somebody reads in a list of thousands.
	if len(s.Description) > 200 {
		t.Errorf("description is %d characters; registries truncate", len(s.Description))
	}
}

