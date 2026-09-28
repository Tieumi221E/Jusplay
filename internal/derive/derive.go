// Package derive turns a verified snapshot into the V1 threads array that
// niconicomments reads, plus a report of everything the derivation saw.
//
// Rules (derivation and identity):
//   - identity is (threadId, fork, comment.id); without id it falls back to
//     (threadId, fork, no, postedAt, vposMs, body) and is counted as low
//     confidence;
//   - the first occurrence in capture order wins; later identical copies are
//     duplicates, copies differing only in volatile fields are drift, any
//     other difference is a conflict, and all are reported with locations;
//   - comments keep their original bytes (field order, unknown fields,
//     number text); output order is first appearance, never re-sorted;
//   - comments that niconicomments would reject are excluded from the
//     playback file and listed as invalid; the snapshot keeps them.
//
// Output is a pure function of the snapshot bytes and options.
package derive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Tieumi221E/Jusplay/internal/importzouryou"
	"github.com/Tieumi221E/Jusplay/internal/legacyxml"
	"github.com/Tieumi221E/Jusplay/internal/snapshot"
)

// Volatile fields may legitimately change between captures of the same comment.
var volatile = map[string]bool{"nicoruCount": true, "score": true, "nicoruId": true, "isMyPost": true}

const maxSafe = 1<<53 - 1

type Options struct {
	// Forks to include in the playback file; nil means all.
	Forks []string
}

type Location struct {
	Response string `json:"response"`
	Thread   int    `json:"thread"`
	Comment  int    `json:"comment"`
}

func (l Location) String() string { return fmt.Sprintf("%s#%d/%d", l.Response, l.Thread, l.Comment) }

type Issue struct {
	Location Location `json:"location"`
	Reason   string   `json:"reason"`
}

type Collision struct {
	Key    string   `json:"key"`
	First  Location `json:"first"`
	Other  Location `json:"other"`
	Fields []string `json:"fields"`
}

type ThreadReport struct {
	ThreadID              string   `json:"threadId"`
	Fork                  string   `json:"fork"`
	Included              bool     `json:"included"`
	CommentCountObserved  []string `json:"commentCountObserved"`
	Comments              int      `json:"comments"`
	Duplicates            int      `json:"duplicates"`
	Drift                 int      `json:"drift"`
	Conflicts             int      `json:"conflicts"`
	Invalid               int      `json:"invalid"`
	LowConfidenceIdentity int      `json:"lowConfidenceIdentity"`
	// NumberRange and MissingNumbers describe comment numbers (no): gaps
	// inside the range are comments deleted or not retrieved.
	NumberRange    [2]int64 `json:"numberRange"`
	MissingNumbers int64    `json:"missingNumbers"`
}

type LegacyReport struct {
	Chats            int              `json:"chats"`
	Deleted          int              `json:"deletedExcluded"`
	ForkSources      map[string]int   `json:"forkSources"`
	DistinctThreads  int              `json:"distinctThreadValues"`
	DateUsecAllZero  bool             `json:"dateUsecAllZero"`
	AnonymityValues  map[string]int   `json:"anonymityValues"`
	PremiumValues    map[string]int   `json:"premiumValues"`
	UnknownAttrs     map[string]int   `json:"unknownAttributes"`
	OtherElements    map[string]int   `json:"otherElements"`
	ExportLeafCounts map[string]int64 `json:"exportLeafCounts"`
}

type Report struct {
	Schema         string         `json:"schema"`
	SnapshotID     string         `json:"snapshotId"`
	VideoID        string         `json:"videoId"`
	Origin         string         `json:"origin"`
	CaptureStatus  string         `json:"captureStatus"`
	ForksSelected  []string       `json:"forksSelected"`
	Threads        []ThreadReport `json:"threads"`
	Output         int            `json:"outputComments"`
	ResponseErrors []Issue        `json:"responseErrors"`
	Invalid        []Issue        `json:"invalid"`
	Warnings       []Issue        `json:"warnings"`
	Conflicts      []Collision    `json:"conflicts"`
	Drift          []Collision    `json:"drift"`
	Commands       map[string]int `json:"commands"`
	NicoScript     map[string]int `json:"nicoScript"`
	Multiline      int            `json:"multilineComments"`
	Legacy         *LegacyReport  `json:"legacy,omitempty"`
}

type entry struct {
	raw     json.RawMessage // compacted original bytes
	content string          // canonical form without volatile fields
	fields  map[string]json.RawMessage
	loc     Location
}

type thread struct {
	id      json.RawMessage
	idText  string
	fork    string
	counts  []string
	entries []*entry
	byKey   map[string]*entry
	rep     *ThreadReport
}

type deriver struct {
	rep     *Report
	threads []*thread
	byTK    map[string]*thread
}

func (d *deriver) thread(id json.RawMessage, idText, fork string) *thread {
	k := idText + "\x00" + fork
	if t := d.byTK[k]; t != nil {
		return t
	}
	t := &thread{id: id, idText: idText, fork: fork, byKey: map[string]*entry{},
		rep: &ThreadReport{ThreadID: idText, Fork: fork, CommentCountObserved: []string{}}}
	d.byTK[k] = t
	d.threads = append(d.threads, t)
	return t
}

func (t *thread) observeCount(raw string) {
	for _, c := range t.counts {
		if c == raw {
			return
		}
	}
	t.counts = append(t.counts, raw)
	t.rep.CommentCountObserved = append(t.rep.CommentCountObserved, raw)
}

// Derive produces the playback JSON and the derivation report.
func Derive(s *snapshot.Snapshot, opt Options) ([]byte, *Report, error) {
	rep := &Report{
		Schema:         "jusplay-derive-report/0",
		SnapshotID:     s.Capture.SnapshotID,
		VideoID:        s.Capture.VideoID,
		Origin:         s.Capture.Origin,
		CaptureStatus:  s.Report.Status,
		ForksSelected:  opt.Forks,
		ResponseErrors: []Issue{}, Invalid: []Issue{}, Warnings: []Issue{},
		Conflicts: []Collision{}, Drift: []Collision{},
		Commands: map[string]int{}, NicoScript: map[string]int{},
	}
	if rep.ForksSelected == nil {
		rep.ForksSelected = []string{}
	}
	d := &deriver{rep: rep, byTK: map[string]*thread{}}
	for i, r := range s.Capture.Responses {
		var err error
		switch r.Format {
		case snapshot.FormatV1:
			err = d.addV1(r.Path, s.Bodies[i])
		case snapshot.FormatLegacyXML:
			err = d.addXML(r.Path, s.Bodies[i])
		case importzouryou.Format:
			err = d.addZouryou(r.Path, s.Bodies[i])
		default:
			err = fmt.Errorf("unsupported format %q", r.Format)
		}
		if err != nil {
			rep.ResponseErrors = append(rep.ResponseErrors, Issue{Location{Response: r.Path, Thread: -1, Comment: -1}, err.Error()})
		}
	}

	include := func(fork string) bool {
		if opt.Forks == nil {
			return true
		}
		for _, f := range opt.Forks {
			if f == fork {
				return true
			}
		}
		return false
	}
	var out bytes.Buffer
	out.WriteByte('[')
	first := true
	for _, t := range d.threads {
		t.rep.Comments = len(t.entries)
		t.numberGaps()
		t.rep.Included = include(t.fork)
		rep.Threads = append(rep.Threads, *t.rep)
		if !t.rep.Included {
			continue
		}
		if !first {
			out.WriteByte(',')
		}
		first = false
		count := "0"
		if len(t.counts) > 0 {
			count = t.counts[0]
		}
		fork, _ := json.Marshal(t.fork)
		fmt.Fprintf(&out, `{"id":%s,"fork":%s,"commentCount":%s,"comments":[`, t.id, fork, count)
		for j, e := range t.entries {
			if j > 0 {
				out.WriteByte(',')
			}
			out.Write(e.raw)
			rep.Output++
		}
		out.WriteString("]}")
	}
	out.WriteString("]\n")
	if rep.Threads == nil {
		rep.Threads = []ThreadReport{}
	}
	return out.Bytes(), rep, nil
}

type v1Envelope struct {
	Meta struct {
		Status json.Number `json:"status"`
	} `json:"meta"`
	Data *struct {
		Threads []struct {
			ID           json.RawMessage   `json:"id"`
			Fork         *string           `json:"fork"`
			CommentCount json.RawMessage   `json:"commentCount"`
			Comments     []json.RawMessage `json:"comments"`
		} `json:"threads"`
	} `json:"data"`
}

func decodeStrict(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	return dec.Decode(v)
}

func (d *deriver) addV1(path string, body []byte) error {
	var env v1Envelope
	if err := decodeStrict(body, &env); err != nil {
		return fmt.Errorf("not JSON: %w", err)
	}
	if env.Meta.Status != "200" {
		return fmt.Errorf("meta.status=%s", env.Meta.Status)
	}
	if env.Data == nil {
		return fmt.Errorf("no data")
	}
	for ti, th := range env.Data.Threads {
		loc := Location{Response: path, Thread: ti, Comment: -1}
		idText, ok := threadIDText(th.ID)
		if !ok || th.Fork == nil || *th.Fork == "" {
			d.rep.ResponseErrors = append(d.rep.ResponseErrors, Issue{loc, "thread without usable id/fork"})
			continue
		}
		t := d.thread(compact(th.ID), idText, *th.Fork)
		if len(th.CommentCount) > 0 {
			if _, err := safeInt(th.CommentCount, 0); err != nil {
				d.rep.ResponseErrors = append(d.rep.ResponseErrors, Issue{loc, "commentCount: " + err.Error()})
			} else {
				t.observeCount(string(compact(th.CommentCount)))
			}
		}
		for ci, c := range th.Comments {
			loc.Comment = ci
			d.addComment(t, compact(c), loc)
		}
	}
	return nil
}

// addZouryou reads a コメント増量 JSON export: V1 threads it assembled,
// the viewer comments merged under one fork. Thread ids are the export's.
func (d *deriver) addZouryou(path string, body []byte) error {
	ts, err := importzouryou.Parse(body)
	if err != nil {
		return err
	}
	for ti, th := range ts {
		idText, ok := threadIDText(th.ID)
		if !ok {
			idText = fmt.Sprint(ti)
		}
		t := d.thread(compact(th.ID), idText, importzouryou.ForkOf(th.Fork))
		t.observeCount(fmt.Sprint(len(th.Comments)))
		for ci, c := range th.Comments {
			d.addComment(t, compact(c), Location{Response: path, Thread: ti, Comment: ci})
		}
	}
	return nil
}

func (t *thread) numberGaps() {
	nos := make([]int64, 0, len(t.entries))
	for _, e := range t.entries {
		var n int64
		if json.Unmarshal(e.fields["no"], &n) == nil {
			nos = append(nos, n)
		}
	}
	if len(nos) == 0 {
		return
	}
	sort.Slice(nos, func(i, j int) bool { return nos[i] < nos[j] })
	var missing int64
	for i := 1; i < len(nos); i++ {
		if d := nos[i] - nos[i-1]; d > 1 {
			missing += d - 1
		}
	}
	t.rep.NumberRange = [2]int64{nos[0], nos[len(nos)-1]}
	t.rep.MissingNumbers = missing
}

func (d *deriver) addXML(path string, body []byte) error {
	doc, err := legacyxml.Parse(body)
	if err != nil {
		return err
	}
	lr := &LegacyReport{
		Chats: len(doc.Chats), ForkSources: map[string]int{},
		AnonymityValues: map[string]int{}, PremiumValues: map[string]int{},
		UnknownAttrs: map[string]int{}, OtherElements: doc.Other, ExportLeafCounts: doc.Leaf,
		DateUsecAllZero: true,
	}
	d.rep.Legacy = lr
	threadVals := map[string]bool{}
	for i, chat := range doc.Chats {
		loc := Location{Response: path, Thread: 0, Comment: i}
		v, _ := chat.Attr("anonymity")
		lr.AnonymityValues[v]++
		v, _ = chat.Attr("premium")
		lr.PremiumValues[v]++
		if u, _ := chat.Attr("date_usec"); strings.Trim(u, "0") != "" {
			lr.DateUsecAllZero = false
		}
		cv, err := legacyxml.Convert(chat)
		if err != nil {
			d.rep.Invalid = append(d.rep.Invalid, Issue{loc, err.Error()})
			continue
		}
		threadVals[cv.ThreadID] = true
		for _, a := range cv.UnknownAttrs {
			lr.UnknownAttrs[a]++
		}
		lr.ForkSources[string(cv.ForkSource)]++
		if cv.Deleted {
			lr.Deleted++
			continue
		}
		idJSON, _ := json.Marshal(cv.ThreadID)
		t := d.thread(idJSON, cv.ThreadID, cv.Fork)
		raw, err := legacyxml.MarshalComment(cv.Comment)
		if err != nil {
			return err
		}
		d.addComment(t, raw, loc)
	}
	lr.DistinctThreads = len(threadVals)
	for _, t := range d.threads {
		t.observeCount(fmt.Sprint(len(t.entries)))
	}
	return nil
}

func (d *deriver) addComment(t *thread, raw json.RawMessage, loc Location) {
	fields, key, low, reason := checkComment(t, raw)
	if reason != "" {
		d.rep.Invalid = append(d.rep.Invalid, Issue{loc, reason})
		t.rep.Invalid++
		return
	}
	if low {
		t.rep.LowConfidenceIdentity++
	}
	e := &entry{raw: raw, fields: fields, content: canonical(fields, true), loc: loc}
	prev := t.byKey[key]
	if prev == nil {
		t.byKey[key] = e
		t.entries = append(t.entries, e)
		d.countContent(t, fields, loc)
		return
	}
	switch {
	case bytes.Equal(prev.raw, raw):
		t.rep.Duplicates++
	case prev.content == e.content:
		t.rep.Drift++
		d.rep.Drift = append(d.rep.Drift, Collision{key, prev.loc, loc, diffFields(prev.fields, fields)})
	default:
		t.rep.Conflicts++
		d.rep.Conflicts = append(d.rep.Conflicts, Collision{key, prev.loc, loc, diffFields(prev.fields, fields)})
	}
}

func (d *deriver) countContent(t *thread, fields map[string]json.RawMessage, loc Location) {
	var cmds []string
	_ = json.Unmarshal(fields["commands"], &cmds)
	for _, c := range cmds {
		d.rep.Commands[c]++
	}
	var body string
	_ = json.Unmarshal(fields["body"], &body)
	if strings.ContainsAny(body, "\n\r") {
		d.rep.Multiline++
	}
	if t.fork == "owner" && (strings.HasPrefix(body, "@") || strings.HasPrefix(body, "＠")) {
		name := strings.FieldsFunc(body, func(r rune) bool { return r == ' ' || r == '　' || r == '\n' })[0]
		d.rep.NicoScript[name]++
	}
	var posted string
	_ = json.Unmarshal(fields["postedAt"], &posted)
	if _, err := time.Parse(time.RFC3339Nano, posted); err != nil {
		d.rep.Warnings = append(d.rep.Warnings, Issue{loc, "postedAt not RFC 3339; niconicomments may skip this comment"})
	}
}

// checkComment validates the fields niconicomments' V1 schema requires and
// returns the identity key.
func checkComment(t *thread, raw json.RawMessage) (fields map[string]json.RawMessage, key string, low bool, reason string) {
	if err := decodeStrict(raw, &fields); err != nil {
		return nil, "", false, "not an object"
	}
	need := func(name string, check func(json.RawMessage) error) {
		if reason != "" {
			return
		}
		v, ok := fields[name]
		if !ok {
			reason = "missing " + name
			return
		}
		if err := check(v); err != nil {
			reason = name + ": " + err.Error()
		}
	}
	// Go decodes null into strings, bools and slices without error; the JS
	// schema does not accept it.
	str := func(v json.RawMessage) error {
		var s string
		if isNull(v) {
			return fmt.Errorf("null")
		}
		return decodeStrict(v, &s)
	}
	boolean := func(v json.RawMessage) error {
		var b bool
		if isNull(v) {
			return fmt.Errorf("null")
		}
		return decodeStrict(v, &b)
	}
	nonNeg := func(v json.RawMessage) error { _, err := safeInt(v, 0); return err }
	anyInt := func(v json.RawMessage) error { _, err := safeInt(v, -maxSafe); return err }
	need("id", str)
	need("no", nonNeg)
	need("vposMs", anyInt)
	need("body", str)
	need("commands", func(v json.RawMessage) error {
		var s []*string
		if isNull(v) {
			return fmt.Errorf("null")
		}
		if err := decodeStrict(v, &s); err != nil {
			return err
		}
		for _, c := range s {
			if c == nil {
				return fmt.Errorf("null command")
			}
		}
		return nil
	})
	need("userId", str)
	need("isPremium", boolean)
	need("score", anyInt)
	need("postedAt", str)
	need("nicoruCount", nonNeg)
	need("nicoruId", func(v json.RawMessage) error {
		if isNull(v) {
			return nil
		}
		return str(v)
	})
	need("source", str)
	need("isMyPost", boolean)
	if reason != "" {
		return nil, "", false, reason
	}
	var id string
	_ = json.Unmarshal(fields["id"], &id)
	prefix := t.idText + "\x00" + t.fork + "\x00"
	if id != "" {
		return fields, prefix + "id:" + id, false, ""
	}
	return fields, prefix + "fb:" + string(fields["no"]) + "\x00" + string(fields["postedAt"]) + "\x00" +
		string(fields["vposMs"]) + "\x00" + string(fields["body"]), true, ""
}

func isNull(v json.RawMessage) bool { return string(bytes.TrimSpace(v)) == "null" }

func safeInt(v json.RawMessage, min int64) (int64, error) {
	var n json.Number
	// json.Number also accepts quoted numbers; JS typeof would say "string".
	if t := bytes.TrimSpace(v); len(t) == 0 || t[0] == '"' || isNull(t) {
		return 0, fmt.Errorf("not a number")
	}
	if err := decodeStrict(v, &n); err != nil {
		return 0, fmt.Errorf("not a number")
	}
	s := n.String()
	if strings.ContainsAny(s, ".eE") {
		f, err := n.Float64()
		if err != nil || f != math.Trunc(f) {
			return 0, fmt.Errorf("not an integer: %s", s)
		}
		if f < float64(min) || f > maxSafe {
			return 0, fmt.Errorf("out of safe range: %s", s)
		}
		return int64(f), nil
	}
	x, err := n.Int64()
	if err != nil || x < min || x > maxSafe {
		return 0, fmt.Errorf("out of safe range: %s", s)
	}
	return x, nil
}

func threadIDText(v json.RawMessage) (string, bool) {
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		return s, s != ""
	}
	if _, err := safeInt(v, 0); err == nil {
		return string(compact(v)), true
	}
	return "", false
}

func compact(v json.RawMessage) json.RawMessage {
	var b bytes.Buffer
	if err := json.Compact(&b, v); err != nil {
		return v
	}
	return b.Bytes()
}

func canonical(fields map[string]json.RawMessage, dropVolatile bool) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		if dropVolatile && volatile[k] {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte(0)
		b.Write(compact(fields[k]))
		b.WriteByte(0)
	}
	return b.String()
}

func diffFields(a, b map[string]json.RawMessage) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range []map[string]json.RawMessage{a, b} {
		for k := range m {
			if seen[k] {
				continue
			}
			seen[k] = true
			if !bytes.Equal(compact(a[k]), compact(b[k])) {
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}
