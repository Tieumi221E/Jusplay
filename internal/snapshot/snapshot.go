// Package snapshot reads, verifies and writes v0 snapshot directories:
//
//	capture.json
//	report.json
//	responses/NNNNNN.json|xml
//
// The response bytes are the source of truth; everything else is metadata
// about them.
package snapshot

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing/fstest"

	"github.com/Tieumi221E/Jusplay/schema"
)

const (
	CaptureFile = "capture.json"
	ReportFile  = "report.json"

	FormatV1        = "nv-comment-v1"
	FormatLegacyXML = "legacy-xml"
)

type Target struct {
	ThreadID string         `json:"threadId"`
	Fork     string         `json:"fork"`
	Extra    map[string]any `json:"extra,omitempty"`
}

type Window struct {
	When *int64 `json:"when,omitempty"`
}

type Response struct {
	Path       string  `json:"path"`
	SHA256     string  `json:"sha256"`
	Bytes      int64   `json:"bytes"`
	Format     string  `json:"format"`
	FetchedAt  *string `json:"fetchedAt"`
	Target     *Target `json:"target"`
	Window     *Window `json:"window"`
	HTTPStatus *int    `json:"httpStatus"`
	APIStatus  *int    `json:"apiStatus,omitempty"`
}

type Collector struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Capture struct {
	Schema             string          `json:"schema"`
	SnapshotID         string          `json:"snapshotId"`
	VideoID            string          `json:"videoId"`
	Origin             string          `json:"origin"`
	CapturedFrom       *string         `json:"capturedFrom"`
	CapturedTo         *string         `json:"capturedTo"`
	Collector          Collector       `json:"collector"`
	Targets            []Target        `json:"targets"`
	PageDisplayProfile json.RawMessage `json:"pageDisplayProfile"`
	Responses          []Response      `json:"responses"`
	Extra              map[string]any  `json:"extra,omitempty"`
}

type TargetReport struct {
	ThreadID            string  `json:"threadId"`
	Fork                string  `json:"fork"`
	Status              string  `json:"status"`
	Requests            int     `json:"requests"`
	Failures            int     `json:"failures"`
	APICommentCount     *int64  `json:"apiCommentCount,omitempty"`
	TerminationEvidence *string `json:"terminationEvidence,omitempty"`
}

type Report struct {
	Schema  string         `json:"schema"`
	Status  string         `json:"status"`
	Targets []TargetReport `json:"targets"`
	Lossy   []string       `json:"lossy"`
	Notes   []string       `json:"notes,omitempty"`
}

// Snapshot is a loaded and verified snapshot.
type Snapshot struct {
	Capture    Capture
	Report     Report
	CaptureRaw []byte
	ReportRaw  []byte
	// Bodies holds response bytes in capture order, parallel to Capture.Responses.
	Bodies [][]byte
}

// Load reads a snapshot from fsys and verifies schemas, every response's
// byte count and SHA-256, and that no unreferenced files are present.
func Load(fsys fs.FS) (*Snapshot, error) {
	s := &Snapshot{}
	var err error
	if s.CaptureRaw, err = fs.ReadFile(fsys, CaptureFile); err != nil {
		return nil, err
	}
	if s.ReportRaw, err = fs.ReadFile(fsys, ReportFile); err != nil {
		return nil, err
	}
	if err := schema.Validate(schema.Capture, s.CaptureRaw); err != nil {
		return nil, err
	}
	if err := schema.Validate(schema.Report, s.ReportRaw); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(s.CaptureRaw, &s.Capture); err != nil {
		return nil, fmt.Errorf("capture.json: %w", err)
	}
	if err := json.Unmarshal(s.ReportRaw, &s.Report); err != nil {
		return nil, fmt.Errorf("report.json: %w", err)
	}

	var errs []error
	referenced := map[string]bool{CaptureFile: true, ReportFile: true}
	for i, r := range s.Capture.Responses {
		if referenced[r.Path] {
			errs = append(errs, fmt.Errorf("responses[%d]: path %s listed twice", i, r.Path))
			s.Bodies = append(s.Bodies, nil)
			continue
		}
		referenced[r.Path] = true
		b, err := fs.ReadFile(fsys, r.Path)
		if err != nil {
			errs = append(errs, fmt.Errorf("responses[%d]: %w", i, err))
			s.Bodies = append(s.Bodies, nil)
			continue
		}
		if int64(len(b)) != r.Bytes {
			errs = append(errs, fmt.Errorf("%s: %d bytes, capture.json says %d", r.Path, len(b), r.Bytes))
		}
		if got := SHA256Hex(b); got != r.SHA256 {
			errs = append(errs, fmt.Errorf("%s: sha256 %s, capture.json says %s", r.Path, got, r.SHA256))
		}
		s.Bodies = append(s.Bodies, b)
	}
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && !referenced[p] {
			errs = append(errs, fmt.Errorf("unreferenced file %s", p))
		}
		return nil
	})
	if err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return s, nil
}

// Files returns every file of the snapshot as path -> bytes, suitable for
// packing. Paths use forward slashes.
func (s *Snapshot) Files() map[string][]byte {
	m := map[string][]byte{CaptureFile: s.CaptureRaw, ReportFile: s.ReportRaw}
	for i, r := range s.Capture.Responses {
		m[r.Path] = s.Bodies[i]
	}
	return m
}

// SortedPaths returns the keys of files in byte order.
func SortedPaths(files map[string][]byte) []string {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// MarshalDoc renders a metadata document the way snapshots store it:
// two-space indented JSON with a trailing newline.
func MarshalDoc(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Build fills response hashes, renders the documents and returns every
// file of the snapshot (path -> bytes), validated like a loaded one.
func Build(capture Capture, report Report, bodies [][]byte) (map[string][]byte, error) {
	if len(bodies) != len(capture.Responses) {
		return nil, fmt.Errorf("%d bodies for %d responses", len(bodies), len(capture.Responses))
	}
	for i := range capture.Responses {
		capture.Responses[i].Bytes = int64(len(bodies[i]))
		capture.Responses[i].SHA256 = SHA256Hex(bodies[i])
	}
	cb, err := MarshalDoc(capture)
	if err != nil {
		return nil, err
	}
	rb, err := MarshalDoc(report)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{CaptureFile: cb, ReportFile: rb}
	for i, r := range capture.Responses {
		files[r.Path] = bodies[i]
	}
	return files, nil
}

// InMemory builds a snapshot without touching the disk (for comment files
// found next to a video at play time).
func InMemory(capture Capture, report Report, bodies [][]byte) (*Snapshot, error) {
	files, err := Build(capture, report, bodies)
	if err != nil {
		return nil, err
	}
	m := fstest.MapFS{}
	for p, b := range files {
		m[p] = &fstest.MapFile{Data: b}
	}
	return Load(m)
}

// WriteDir writes a new snapshot directory. dir must not exist yet.
func WriteDir(dir string, capture Capture, report Report, bodies [][]byte) error {
	files, err := Build(capture, report, bodies)
	if err != nil {
		return err
	}
	if _, err := Load(mapFS(files)); err != nil {
		return err
	}
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("%s already exists", dir)
	}
	if err := os.MkdirAll(filepath.Join(dir, "responses"), 0o755); err != nil {
		return err
	}
	for p, b := range files {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(p)), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func mapFS(files map[string][]byte) fstest.MapFS {
	m := fstest.MapFS{}
	for p, b := range files {
		m[p] = &fstest.MapFile{Data: b}
	}
	return m
}

func SHA256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// NewUUID returns a random RFC 4122 version 4 UUID.
func NewUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
