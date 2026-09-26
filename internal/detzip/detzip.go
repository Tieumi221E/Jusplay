// Package detzip writes byte-for-byte reproducible ZIP archives: entries
// in path order, a fixed timestamp, fixed permissions and Deflate.
// Reproducibility holds for a given Go toolchain version (compress/flate).
package detzip

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"time"
)

// Epoch is the timestamp stored for every entry (the DOS minimum).
var Epoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

// Write archives files (slash-separated path -> bytes).
func Write(files map[string][]byte) ([]byte, error) {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, p := range paths {
		if !fs.ValidPath(p) {
			return nil, fmt.Errorf("invalid path %q", p)
		}
		h := &zip.FileHeader{Name: p, Method: zip.Deflate}
		// Setting only the legacy DOS fields keeps the extended-timestamp
		// extra field out of the archive.
		h.ModifiedDate = 1<<5 | 1 // 1980-01-01
		h.ModifiedTime = 0
		h.SetMode(0o644)
		w, err := zw.CreateHeader(h)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(files[p]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Limits for archives read from files (a comment capture is a few MB; a
// crafted archive can claim anything). archive/zip itself refuses an entry
// that inflates past its declared size, so checking the declared sizes
// bounds what reading can allocate.
const (
	MaxEntries   = 100_000
	MaxEntrySize = 512 << 20
	MaxTotalSize = 2 << 30
)

// Open returns the archive as a read-only file system, after checking its
// declared sizes against the limits.
func Open(b []byte) (fs.FS, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, err
	}
	if len(zr.File) > MaxEntries {
		return nil, fmt.Errorf("archive has %d entries (limit %d)", len(zr.File), MaxEntries)
	}
	var total uint64
	for _, f := range zr.File {
		if f.UncompressedSize64 > MaxEntrySize {
			return nil, fmt.Errorf("archive entry %s is %d bytes unpacked (limit %d)", f.Name, f.UncompressedSize64, MaxEntrySize)
		}
		if total += f.UncompressedSize64; total > MaxTotalSize {
			return nil, fmt.Errorf("archive unpacks to more than %d bytes", MaxTotalSize)
		}
	}
	return zr, nil
}

// ReadAll returns every regular file in fsys.
func ReadAll(fsys fs.FS) (map[string][]byte, error) {
	out := map[string][]byte{}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		f, err := fsys.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		out[p] = b
		return err
	})
	return out, err
}
