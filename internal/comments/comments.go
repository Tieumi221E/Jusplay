// Package comments opens comment data from any source jusplay accepts:
// a snapshot directory, a raw snapshot ZIP, a コメント増量 JSON export or a
// legacy XML export. Exports are read into an in-memory snapshot; nothing
// is written.
package comments

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tieumi221E/Jusplay/internal/detzip"
	"github.com/Tieumi221E/Jusplay/internal/importxml"
	"github.com/Tieumi221E/Jusplay/internal/importzouryou"
	"github.com/Tieumi221E/Jusplay/internal/snapshot"
)

// MaxFile bounds a comment file read into memory (the largest real export
// seen is tens of MB).
const MaxFile = 1 << 30

// Version is recorded as the collector version of in-memory imports.
var Version = "dev"

// Open reads path. videoID is used for exports that carry none; when empty
// it is taken from a file named after a Niconico id (so00000001.json).
func Open(path, videoID string) (*snapshot.Snapshot, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return snapshot.Load(os.DirFS(path))
	}
	if st.Size() > MaxFile {
		return nil, fmt.Errorf("%s is %d MB; comment files over %d MB are not read", filepath.Base(path), st.Size()>>20, MaxFile>>20)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if videoID == "" {
		videoID = importzouryou.VideoIDFromName(path)
	}
	name := filepath.Base(path)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".zip":
		zfs, err := detzip.Open(b)
		if err != nil {
			return nil, err
		}
		return snapshot.Load(zfs)
	case ".json":
		return importzouryou.InMemory(b, videoID, name, Version)
	case ".xml":
		return importxml.InMemory(b, videoID, name, Version)
	}
	return nil, fmt.Errorf("%s: not a snapshot directory, .zip, .json or .xml", name)
}

// SameName returns the comment file next to video that shares its name
// (video.mkv -> video.json), or "" if there is none.
func SameName(video string) string {
	p := strings.TrimSuffix(video, filepath.Ext(video)) + ".json"
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
	}
	return ""
}

// ErrNone means no comment source was found.
var ErrNone = errors.New("no comment file")
