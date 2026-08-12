// Package linkmeta is what a link looks like when you show it to somebody.
//
// The Open Graph tags behind a URL — title, description, image, site — cached
// on disk so the same link is not fetched twice. News fills it as it reads
// feeds; social reads it to render a preview when somebody posts a link.
//
// It is here rather than in either of them because it is about *links*, not
// about news or about social. service/social imported service/news for one
// function, LookupMetadata, and that one function made the two services a unit:
// social could not be read, changed or moved without news coming along.
//
// The on-disk path is deliberately still news/metadata/<hash>.json. Every
// instance already has that directory full of cached previews, and moving them
// to match a package name would throw the cache away for no benefit anybody can
// see. Where the files are is where they have always been, not a claim about
// what owns them.
package linkmeta

import (
	"crypto/md5"
	"fmt"
	"path/filepath"

	"mu/internal/data"
)

// Metadata is the Open Graph description of a URL.
//
// The summary fields are the agent's, not Open Graph's: a page's own tags say
// what it claims to be about, and the summary is what a model made of actually
// reading it. Both are about the link, so both live on the same record.
type Metadata struct {
	Created            int64
	Title              string
	Description        string
	Type               string
	Image              string
	Url                string
	Site               string
	Content            string
	Comments           string // Comments/discussion context from any source
	Summary            string // LLM-generated summary for chat context
	SummaryRequestedAt int64  // Last time we requested summary generation
	SummaryAttempts    int    // Number of times we've requested a summary
}

// Path is where a URL's metadata is filed. Hashed, because a URL is not a
// filename: it has slashes, query strings and no length limit.
func Path(uri string) string {
	itemID := fmt.Sprintf("%x", md5.Sum([]byte(uri)))[:16]
	return filepath.Join("news", "metadata", itemID+".json")
}

// Lookup returns the cached metadata for a URL, and whether there was any.
func Lookup(uri string) (*Metadata, bool) {
	var md Metadata
	if err := data.LoadJSON(Path(uri), &md); err != nil {
		return nil, false
	}
	return &md, true
}

// Save writes a URL's metadata to the cache. It returns the error rather than
// logging it, because what to do about a failed cache write is the caller's
// question — for a cache, usually nothing.
func Save(uri string, md *Metadata) error {
	return data.SaveJSON(Path(uri), md)
}
