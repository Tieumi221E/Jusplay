// Package importzouryou turns a コメント増量 JSON export into a v0
// snapshot. The export is a V1 threads array the extension assembled
// itself: every non-owner thread merged into one array (fork
// "comment-zouryou"), owner comments in a second one, after the
// extension's own NG settings. The bytes are kept unchanged; derive reads
// them as format comment-zouryou-json.
package importzouryou

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Tieumi221E/Jusplay/internal/snapshot"
)

// Format is the snapshot response format of these exports.
const Format = "comment-zouryou-json"

var lossy = []string{
	"コメント増量 JSON export, not raw API responses; assembled and paged by the extension",
	"all non-owner threads (main threads and easy) are merged into one array; their fork and thread ids are lost",
	"the extension's NG settings (NG words/commands, NG share score, nicoru, premium filters) applied at export time are not recorded",
	"commentCount is the exported count, not the server's",
	"capture time and page display settings are unknown",
}

type thread struct {
	ID           json.RawMessage   `json:"id"`
	Fork         string            `json:"fork"`
	CommentCount json.RawMessage   `json:"commentCount"`
	Comments     []json.RawMessage `json:"comments"`
}

// Parse reads the export's structure.
func Parse(b []byte) ([]thread, error) {
	var ts []thread
	if err := json.Unmarshal(b, &ts); err != nil {
		return nil, fmt.Errorf("not a threads array: %w", err)
	}
	if len(ts) == 0 {
		return nil, errors.New("no threads")
	}
	for i, t := range ts {
		if t.Fork == "" {
			return nil, fmt.Errorf("thread %d has no fork", i)
		}
	}
	return ts, nil
}

// Import writes a snapshot for the export into dir.
func Import(b []byte, videoID, sourceName, version, dir string) (*snapshot.Capture, error) {
	c, r, err := docs(b, videoID, sourceName, version)
	if err != nil {
		return nil, err
	}
	if err := snapshot.WriteDir(dir, c, r, [][]byte{b}); err != nil {
		return nil, err
	}
	return &c, nil
}

// InMemory reads the export as a snapshot without writing anything.
func InMemory(b []byte, videoID, sourceName, version string) (*snapshot.Snapshot, error) {
	c, r, err := docs(b, videoID, sourceName, version)
	if err != nil {
		return nil, err
	}
	return snapshot.InMemory(c, r, [][]byte{b})
}

// VideoIDFromName returns a Niconico id when the file is named after one
// (so00000001.json), else "".
func VideoIDFromName(name string) string {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	if videoID.MatchString(base) {
		return base
	}
	return ""
}

var videoID = regexp.MustCompile(`^[a-z]{2}[0-9]+$`)

func docs(b []byte, videoID, sourceName, version string) (snapshot.Capture, snapshot.Report, error) {
	ts, err := Parse(b)
	if err != nil {
		return snapshot.Capture{}, snapshot.Report{}, err
	}
	var targets []snapshot.Target
	var reports []snapshot.TargetReport
	notes := []string{}
	for _, t := range ts {
		fork := ForkOf(t.Fork)
		id := string(t.ID)
		targets = append(targets, snapshot.Target{ThreadID: id, Fork: fork, Extra: map[string]any{"exportFork": t.Fork}})
		n := int64(len(t.Comments))
		reports = append(reports, snapshot.TargetReport{ThreadID: id, Fork: fork, Status: "partial", APICommentCount: &n})
		notes = append(notes, fmt.Sprintf("export thread %s (%s): %d comments", id, t.Fork, n))
	}
	c := snapshot.Capture{
		Schema:             "jusplay-capture/0",
		SnapshotID:         snapshot.NewUUID(),
		VideoID:            videoID,
		Origin:             Format,
		Collector:          snapshot.Collector{Name: "jusplay import-zouryou-json", Version: version},
		Targets:            targets,
		PageDisplayProfile: []byte(`"unknown"`),
		Responses:          []snapshot.Response{{Path: "responses/000001.json", Format: Format}},
		Extra:              map[string]any{"sourceFileName": sourceName},
	}
	r := snapshot.Report{
		Schema:  "jusplay-report/0",
		Status:  "partial",
		Targets: reports,
		Lossy:   lossy,
		Notes:   notes,
	}
	return c, r, nil
}

// ForkOf maps the export's fork names: "owner" stays, everything else is
// the merged viewer array, rendered as main.
func ForkOf(exportFork string) string {
	if exportFork == "owner" {
		return "owner"
	}
	return "main"
}
