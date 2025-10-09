package data

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/philippgille/chromem-go"
)

var DB *chromem.DB

func init() {
	dir := os.ExpandEnv("$HOME/.mu")
	path := filepath.Join(dir, "data", "index")

	db, err := chromem.NewPersistentDB(path, false)
	if err != nil {
		panic(err)
	}

	// set db
	DB = db
}

// Save to disk
func Save(key, val string) error {
	dir := os.ExpandEnv("$HOME/.mu")
	path := filepath.Join(dir, "data")
	file := filepath.Join(path, key)
	os.MkdirAll(path, 0700)
	os.WriteFile(file, []byte(val), 0644)
	return nil
}

// Load file from disk
func Load(key string) ([]byte, error) {
	dir := os.ExpandEnv("$HOME/.mu")
	path := filepath.Join(dir, "data")
	file := filepath.Join(path, key)
	return os.ReadFile(file)
}

func SaveJSON(key string, val interface{}) error {
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}

	dir := os.ExpandEnv("$HOME/.mu")
	path := filepath.Join(dir, "data")
	file := filepath.Join(path, key)
	os.MkdirAll(path, 0700)
	os.WriteFile(file, b, 0644)

	return nil
}

// Index content
func Index(id string, md map[string]string, content string) error {
	c, err := DB.GetOrCreateCollection("mu", nil, nil)
	if err != nil {
		return err
	}
	return c.AddDocument(context.TODO(), chromem.Document{
		ID:       id,
		Metadata: md,
		Content:  content,
	})
}

// Indexed document
type Doc struct {
	ID       string
	Metadata map[string]string
	Content  string
}

// Retrieve from index
func Search(q string, limit int, where map[string]string) ([]Doc, error) {
	c, err := DB.GetOrCreateCollection("mu", nil, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.Query(context.TODO(), q, limit, where, nil)
	if err != nil {
		return nil, err
	}
	var docs []Doc
	for _, val := range res {
		docs = append(docs, Doc{
			ID:       val.ID,
			Metadata: val.Metadata,
			Content:  val.Content,
		})
	}
	return docs, nil
}
