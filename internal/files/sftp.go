package files

// The Files namespace projected as a deliberately flat filesystem.
//
// SFTP never sees a path on this machine. Every operation below resolves a
// displayed name to a Files metadata record, and every byte still passes
// through blob.Store. The account came from SSH public-key authentication.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
)

// ServeSFTP serves one authenticated account until its SSH channel closes.
func ServeSFTP(account string, ch io.ReadWriteCloser) error {
	fs := &sftpFiles{account: account}
	server := sftp.NewRequestServer(ch, sftp.Handlers{
		FileGet: fs, FilePut: fs, FileCmd: fs, FileList: fs,
	})
	return server.Serve()
}

type sftpFiles struct{ account string }

type projected struct {
	name string
	file *File
}

func (fs *sftpFiles) entries() []projected {
	stored := List(fs.account) // newest first
	used := map[string]bool{}
	out := make([]projected, 0, len(stored))
	for _, f := range stored {
		name := f.Name
		if used[name] {
			name = disambiguated(f, used)
		}
		used[name] = true
		out = append(out, projected{name: name, file: f})
	}
	return out
}

func disambiguated(f *File, used map[string]bool) string {
	ext := filepath.Ext(f.Name)
	base := strings.TrimSuffix(f.Name, ext)
	name := base + " (" + f.ID + ")" + ext
	for used[name] {
		name = "_" + name
	}
	return name
}

func flatName(p string) (string, error) {
	p = path.Clean("/" + strings.TrimSpace(p))
	if p == "/" {
		return "", nil
	}
	name := strings.TrimPrefix(p, "/")
	if name == "" || strings.Contains(name, "/") || name == "." || name == ".." {
		return "", os.ErrInvalid
	}
	return name, nil
}

func (fs *sftpFiles) find(p string) (*projected, error) {
	name, err := flatName(p)
	if err != nil || name == "" {
		return nil, os.ErrNotExist
	}
	for _, e := range fs.entries() {
		if e.name == name {
			entry := e
			return &entry, nil
		}
	}
	return nil, os.ErrNotExist
}

func (fs *sftpFiles) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	e, err := fs.find(r.Filepath)
	if err != nil {
		return nil, err
	}
	_, raw, err := Get(fs.account, e.file.ID)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(raw), nil
}

func (fs *sftpFiles) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	name, err := flatName(r.Filepath)
	if err != nil || name == "" {
		return nil, os.ErrInvalid
	}
	flags := r.Pflags()
	e, foundErr := fs.find(r.Filepath)
	if foundErr == nil && flags.Creat && flags.Excl {
		return nil, os.ErrExist
	}
	var id string
	var initial []byte
	if foundErr == nil {
		id = e.file.ID
		if !flags.Trunc {
			_, initial, err = Get(fs.account, id)
			if err != nil {
				return nil, err
			}
		}
	} else if !flags.Creat {
		return nil, os.ErrNotExist
	}
	return &sftpWriter{account: fs.account, id: id, name: name, data: initial}, nil
}

type sftpWriter struct {
	account string
	id      string
	name    string
	data    []byte
	closed  bool
	mu      sync.Mutex
}

func (w *sftpWriter) WriteAt(p []byte, off int64) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || off < 0 || off > MaxBytes || len(p) > MaxBytes-int(off) {
		return 0, fmt.Errorf("file exceeds the %d byte limit", MaxBytes)
	}
	end := int(off) + len(p)
	if end > len(w.data) {
		w.data = append(w.data, make([]byte, end-len(w.data))...)
	}
	return copy(w.data[int(off):], p), nil
}

func (w *sftpWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	_, err := Replace(w.account, w.id, w.name, "", w.data)
	return err
}

func (fs *sftpFiles) Filecmd(r *sftp.Request) error {
	switch r.Method {
	case "Remove":
		e, err := fs.find(r.Filepath)
		if err != nil {
			return err
		}
		return Delete(fs.account, e.file.ID)
	case "Rename":
		e, err := fs.find(r.Filepath)
		if err != nil {
			return err
		}
		name, err := flatName(r.Target)
		if err != nil || name == "" {
			return os.ErrInvalid
		}
		if _, err := fs.find(r.Target); err == nil {
			return os.ErrExist
		}
		_, err = Rename(fs.account, e.file.ID, name)
		return err
	default:
		return sftp.ErrSSHFxOpUnsupported
	}
}

func (fs *sftpFiles) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	name, err := flatName(r.Filepath)
	if err != nil {
		return nil, err
	}
	if name == "" {
		if r.Method == "Stat" {
			return fileInfos{dirInfo{}}, nil
		}
		if r.Method != "List" {
			return nil, sftp.ErrSSHFxOpUnsupported
		}
		entries := fs.entries()
		infos := make(fileInfos, 0, len(entries))
		for _, e := range entries {
			infos = append(infos, sftpInfo{name: e.name, file: e.file})
		}
		return infos, nil
	}
	e, err := fs.find(r.Filepath)
	if err != nil {
		return nil, err
	}
	return fileInfos{sftpInfo{name: e.name, file: e.file}}, nil
}

type fileInfos []os.FileInfo

func (f fileInfos) ListAt(dst []os.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(f)) {
		return 0, io.EOF
	}
	n := copy(dst, f[offset:])
	if int(offset)+n >= len(f) {
		return n, io.EOF
	}
	return n, nil
}

type sftpInfo struct {
	name string
	file *File
}

func (f sftpInfo) Name() string       { return f.name }
func (f sftpInfo) Size() int64        { return int64(f.file.Size) }
func (f sftpInfo) Mode() os.FileMode  { return 0600 }
func (f sftpInfo) ModTime() time.Time { return f.file.Created }
func (f sftpInfo) IsDir() bool        { return false }
func (f sftpInfo) Sys() any           { return nil }

type dirInfo struct{}

func (dirInfo) Name() string       { return "/" }
func (dirInfo) Size() int64        { return 0 }
func (dirInfo) Mode() os.FileMode  { return os.ModeDir | 0700 }
func (dirInfo) ModTime() time.Time { return time.Time{} }
func (dirInfo) IsDir() bool        { return true }
func (dirInfo) Sys() any           { return nil }
