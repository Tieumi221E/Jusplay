package player

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Tieumi221E/Jusplay/internal/library"
)

// thumbs keeps one WebP frame per video. The page makes them (the video's
// keyframe a third of the way in, decoded by the engine, 480 px wide,
// WebP quality 0.75; web/src/thumbnailer.ts) and hands them here; the
// server only checks and stores them, so no decoder is needed here.
type thumbs struct {
	// dirFor is where e's thumbnail goes: its media folder's records, or
	// the app's temporary folder (Server.cacheDir).
	dirFor func(library.Entry) string

	mu   sync.Mutex
	have map[string]bool // thumbnail files known to exist
}

func newThumbs(dirFor func(library.Entry) string) *thumbs {
	return &thumbs{dirFor: dirFor, have: map[string]bool{}}
}

// path is keyed by the file's identity, size and time, so a replaced file
// gets a new thumbnail.
func (t *thumbs) path(e library.Entry) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d|%s", e.ID, e.Size, e.ModTime.UnixNano(), thumbVersion)))
	return filepath.Join(t.dirFor(e), hex.EncodeToString(h[:10])+".webp")
}

// thumbVersion changes when the way thumbnails are made changes.
const thumbVersion = "webp75-480-key"

// has reports whether e's thumbnail exists.
func (t *thumbs) has(e library.Entry) bool {
	p := t.path(e)
	t.mu.Lock()
	ok := t.have[p]
	t.mu.Unlock()
	if ok {
		return true
	}
	if _, err := os.Stat(p); err == nil {
		t.mu.Lock()
		t.have[p] = true
		t.mu.Unlock()
		return true
	}
	return false
}

// maxThumb bounds a thumbnail (they are about 9 KB).
const maxThumb = 1 << 20

// put stores a thumbnail made by the page, after checking that it is a WebP
// image of sane size.
func (t *thumbs) put(e library.Entry, b []byte) error {
	if len(b) < 20 || len(b) > maxThumb || string(b[:4]) != "RIFF" || string(b[8:12]) != "WEBP" ||
		int(binary.LittleEndian.Uint32(b[4:8]))+8 != len(b) {
		return errors.New("not a WebP image")
	}
	p := t.path(e)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".partial"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return err
	}
	t.mu.Lock()
	t.have[p] = true
	t.mu.Unlock()
	return nil
}

// prune removes cached files that belong to no current entry (removed
// videos, replaced files, older thumbnail versions).
func (t *thumbs) prune(entries []library.Entry) (removed int) {
	keep := map[string]map[string]bool{} // per folder
	for _, e := range entries {
		p := t.path(e)
		d := filepath.Dir(p)
		if keep[d] == nil {
			keep[d] = map[string]bool{}
		}
		keep[d][filepath.Base(p)] = true
	}
	for d, k := range keep {
		removed += pruneDir(d, k)
	}
	t.mu.Lock()
	t.have = map[string]bool{}
	t.mu.Unlock()
	return removed
}

// pruneDir removes the files in d not in keep (not .partial ones: being
// written right now).
func pruneDir(d string, keep map[string]bool) (removed int) {
	files, err := os.ReadDir(d)
	if err != nil {
		return 0
	}
	for _, f := range files {
		if !f.IsDir() && !keep[f.Name()] && !strings.HasSuffix(f.Name(), ".partial") &&
			os.Remove(filepath.Join(d, f.Name())) == nil {
			removed++
		}
	}
	return removed
}
