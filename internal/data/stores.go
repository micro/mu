package data

// What is on disk, and how big it has got.
//
// Every store here keeps a whole-file blob: load the lot into memory, mutate,
// marshal the lot back. That is fine for a settings file and it is not fine for
// a log nothing prunes, and the difference between the two is invisible until
// somebody goes looking in ~/.mu/data with ls. There was no way to ask the
// running instance, so the answer to "is anything getting big" was nobody's
// job.
//
// A list, largest first, on the page an operator already opens to look at the
// server. It does not decide anything — a file being large is not by itself a
// fault, since the search index and the news feed are supposed to be — but a
// store that has quietly become the largest thing here is worth seeing.

import (
	"mu/internal/dir"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Store is one file in the data directory, or one directory of them.
type Store struct {
	Name  string
	Size  int64
	Files int // more than one when Name is a directory
}

// Stores lists what the data directory holds, largest first.
//
// A subdirectory is one row with its contents summed, because the interesting
// fact about news/metadata is its total, not its ten thousand members.
func Stores() []Store {
	base := dir.Data()

	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}

	var out []Store
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !e.IsDir() {
			info, err := e.Info()
			if err != nil {
				continue
			}
			out = append(out, Store{Name: name, Size: info.Size(), Files: 1})
			continue
		}

		s := Store{Name: name + "/"}
		filepath.WalkDir(filepath.Join(base, name), func(_ string, d os.DirEntry, err error) error { //nolint:errcheck
			if err != nil || d.IsDir() {
				return nil //nolint:nilerr — an unreadable corner is not worth abandoning the count
			}
			if info, err := d.Info(); err == nil {
				s.Size += info.Size()
				s.Files++
			}
			return nil
		})
		out = append(out, s)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Size > out[j].Size })
	return out
}
