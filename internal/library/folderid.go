package library

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// A media folder's own id, kept in its records (.jusplay/folder.json) and
// made the first time it is asked for. Links (jus://play/<id>/…) name a
// folder by it, so they still work when the folder is on another drive
// letter or another computer.

type folderDoc struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
}

func folderFile(dir string) string { return filepath.Join(dir, VaultDir, "folder.json") }

func readFolderID(dir string) string {
	b, err := os.ReadFile(folderFile(dir))
	if err != nil {
		return ""
	}
	var d folderDoc
	if json.Unmarshal(b, &d) != nil {
		return ""
	}
	return d.ID
}

// FolderID is the id of added folder dir, made and kept if it has none yet.
// "" when the folder cannot keep one (not added, or cannot be written).
func (l *Library) FolderID(dir string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	v := l.vaults[strings.ToLower(filepath.Clean(dir))]
	if v == nil {
		return ""
	}
	if id := readFolderID(v.root); id != "" {
		return id
	}
	if l.foldersPath == "" {
		return ""
	}
	var b [8]byte
	rand.Read(b[:])
	id := hex.EncodeToString(b[:])
	doc, _ := json.Marshal(folderDoc{Version: 1, ID: id})
	if err := writeAtomic(folderFile(v.root), doc); err != nil {
		return ""
	}
	hide(v.dir())
	return id
}

// FolderByID is the added folder with this id.
func (l *Library) FolderByID(id string) (string, bool) {
	if id == "" {
		return "", false
	}
	l.mu.Lock()
	roots := make([]string, 0, len(l.vaults))
	for _, v := range l.vaults {
		roots = append(roots, v.root)
	}
	l.mu.Unlock()
	for _, r := range roots {
		if readFolderID(r) == id {
			return r, true
		}
	}
	return "", false
}
