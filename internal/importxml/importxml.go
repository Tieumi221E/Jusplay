// Package importxml turns a legacy XML export into a v0 snapshot. The XML
// bytes are kept unchanged as the snapshot's only response; conversion to
// V1 happens later in derive, so the import can be re-derived when the
// conversion improves.
package importxml

import (
	"fmt"
	"sort"

	"github.com/Tieumi221E/Jusplay/internal/legacyxml"
	"github.com/Tieumi221E/Jusplay/internal/snapshot"
)

// Fixed statements about what any XML export lacks.
var lossy = []string{
	"legacy XML export, not raw API responses; the exporter may have filtered, rewritten or truncated data",
	"comment id is not recorded; identity falls back to (thread, fork, no, postedAt, vposMs, body)",
	"thread fork is not recorded unless a fork attribute is present; other chats are assumed main",
	"score, nicoruId and isMyPost are not recorded unless present; placeholders are used",
	"vpos has 10 ms resolution; the original vposMs is not recoverable",
	"capture time and page display settings are unknown",
}

// Import writes a snapshot for xmlBytes into dir.
func Import(xmlBytes []byte, videoID, sourceName, version, dir string) (*snapshot.Capture, error) {
	c, r, err := docs(xmlBytes, videoID, sourceName, version)
	if err != nil {
		return nil, err
	}
	if err := snapshot.WriteDir(dir, c, r, [][]byte{xmlBytes}); err != nil {
		return nil, err
	}
	return &c, nil
}

// InMemory reads the export as a snapshot without writing anything.
func InMemory(xmlBytes []byte, videoID, sourceName, version string) (*snapshot.Snapshot, error) {
	c, r, err := docs(xmlBytes, videoID, sourceName, version)
	if err != nil {
		return nil, err
	}
	return snapshot.InMemory(c, r, [][]byte{xmlBytes})
}

func docs(xmlBytes []byte, videoID, sourceName, version string) (snapshot.Capture, snapshot.Report, error) {
	doc, err := legacyxml.Parse(xmlBytes)
	if err != nil {
		return snapshot.Capture{}, snapshot.Report{}, fmt.Errorf("parse XML: %w", err)
	}
	type tk struct{ thread, fork string }
	seen := map[tk]bool{}
	var targets []snapshot.Target
	extraLossy := []string{}
	threadVals := map[string]bool{}
	usecAllZero := true
	for _, c := range doc.Chats {
		cv, err := legacyxml.Convert(c)
		if err != nil {
			continue // reported by derive with its location
		}
		threadVals[cv.ThreadID] = true
		if u, _ := c.Attr("date_usec"); u != "" && u != "0" && u != "00000" && u != "000000" {
			usecAllZero = false
		}
		k := tk{cv.ThreadID, cv.Fork}
		if !seen[k] {
			seen[k] = true
			targets = append(targets, snapshot.Target{ThreadID: cv.ThreadID, Fork: cv.Fork})
		}
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].ThreadID != targets[j].ThreadID {
			return targets[i].ThreadID < targets[j].ThreadID
		}
		return targets[i].Fork < targets[j].Fork
	})
	if len(threadVals) == 1 && len(doc.Chats) > 1 {
		extraLossy = append(extraLossy, "every chat has the same thread value; the exporter may have rewritten it")
	}
	if usecAllZero && len(doc.Chats) > 0 {
		extraLossy = append(extraLossy, "date_usec is zero for every chat; sub-second post times are lost")
	}

	var reports []snapshot.TargetReport
	for _, t := range targets {
		tr := snapshot.TargetReport{ThreadID: t.ThreadID, Fork: t.Fork, Status: "partial"}
		if n, ok := doc.Leaf[t.ThreadID]; ok && len(targets) == 1 {
			tr.APICommentCount = &n
		}
		reports = append(reports, tr)
	}
	if targets == nil {
		targets = []snapshot.Target{}
		reports = []snapshot.TargetReport{}
	}

	c := snapshot.Capture{
		Schema:             "jusplay-capture/0",
		SnapshotID:         snapshot.NewUUID(),
		VideoID:            videoID,
		Origin:             snapshot.FormatLegacyXML,
		Collector:          snapshot.Collector{Name: "jusplay import-xml", Version: version},
		Targets:            targets,
		PageDisplayProfile: []byte(`"unknown"`),
		Responses: []snapshot.Response{{
			Path:   "responses/000001.xml",
			Format: snapshot.FormatLegacyXML,
		}},
		Extra: map[string]any{"sourceFileName": sourceName},
	}
	r := snapshot.Report{
		Schema:  "jusplay-report/0",
		Status:  "partial",
		Targets: reports,
		Lossy:   append(append([]string{}, lossy...), extraLossy...),
		Notes:   []string{"apiCommentCount is the export's own <leaf>/<global_num_res> value, not verified against the API"},
	}
	return c, r, nil
}
