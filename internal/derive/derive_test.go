package derive

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Tieumi221E/Jusplay/internal/importxml"
	"github.com/Tieumi221E/Jusplay/internal/importzouryou"
	"github.com/Tieumi221E/Jusplay/internal/snapshot"
)

func loadFixture(t *testing.T) *snapshot.Snapshot {
	t.Helper()
	s, err := snapshot.Load(os.DirFS("../../testdata/v1-basic"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type outThread struct {
	ID           json.RawMessage   `json:"id"`
	Fork         string            `json:"fork"`
	CommentCount json.RawMessage   `json:"commentCount"`
	Comments     []json.RawMessage `json:"comments"`
}

func ids(t *testing.T, cs []json.RawMessage) []string {
	var out []string
	for _, c := range cs {
		var v struct{ ID string }
		if err := json.Unmarshal(c, &v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v.ID)
	}
	return out
}

func TestDeriveV1(t *testing.T) {
	s := loadFixture(t)
	out, rep, err := Derive(s, Options{})
	if err != nil {
		t.Fatal(err)
	}
	again, _, _ := Derive(loadFixture(t), Options{})
	if !bytes.Equal(out, again) {
		t.Fatal("derive is not deterministic")
	}

	var threads []outThread
	if err := json.Unmarshal(out, &threads); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, th := range threads {
		got = append(got, strings.Trim(string(th.ID), `"`)+"/"+th.Fork)
	}
	if strings.Join(got, " ") != "1000/owner 1001/main 1001/easy" {
		t.Fatalf("thread order %v", got)
	}
	// easy's m-1 must not merge with main's m-1.
	if n := len(threads[2].Comments); n != 1 {
		t.Fatalf("easy comments %d", n)
	}
	// First-appearance order; m-0 from the older window comes last; invalid excluded.
	if g := strings.Join(ids(t, threads[1].Comments), ","); g != "m-1,m-2,m-3,m-4,,m-9,m-0" {
		t.Fatalf("main ids %s", g)
	}
	if string(threads[1].CommentCount) != "12" {
		t.Fatalf("commentCount %s, want first observed 12", threads[1].CommentCount)
	}

	// Original bytes survive: unknown field, number text, escapes, command order.
	for _, want := range []string{
		`"futureField":{"x":1.50}`,
		`"score":-1000`,
		`"body":"<img src=x onerror=alert(1)> &amp; \u0000 😀"`,
		`"commands":["nico:flash","#ff00ff","unknowncmd"]`,
		`"vposMs":-500`,
	} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("output lacks %s", want)
		}
	}
	if bytes.Contains(out, []byte("changed body")) {
		t.Error("conflicting later copy replaced the first")
	}

	main := rep.Threads[1]
	if main.Duplicates != 1 || main.Drift != 1 || main.Conflicts != 1 || main.Invalid != 3 || main.LowConfidenceIdentity != 1 {
		t.Errorf("main report %+v", main)
	}
	if strings.Join(main.CommentCountObserved, ",") != "12,13" {
		t.Errorf("observed counts %v", main.CommentCountObserved)
	}
	if len(rep.Drift) != 1 || strings.Join(rep.Drift[0].Fields, ",") != "nicoruCount" {
		t.Errorf("drift %+v", rep.Drift)
	}
	if len(rep.Conflicts) != 1 || strings.Join(rep.Conflicts[0].Fields, ",") != "body" ||
		rep.Conflicts[0].First.Response != "responses/000001.json" || rep.Conflicts[0].Other.Response != "responses/000002.json" {
		t.Errorf("conflicts %+v", rep.Conflicts)
	}
	reasons := map[string]bool{}
	for _, i := range rep.Invalid {
		reasons[i.Reason] = true
	}
	for _, r := range []string{"no: not a number", "missing isMyPost", "nicoruCount: out of safe range: 9007199254740993"} {
		if !reasons[r] {
			t.Errorf("missing invalid reason %q in %v", r, rep.Invalid)
		}
	}
	if len(rep.ResponseErrors) != 1 || !strings.Contains(rep.ResponseErrors[0].Reason, "403") {
		t.Errorf("response errors %+v", rep.ResponseErrors)
	}
	if len(rep.Warnings) != 1 || rep.Warnings[0].Location.Comment != 8 {
		t.Errorf("warnings %+v", rep.Warnings)
	}
	if rep.NicoScript["@デフォルト"] != 1 || rep.Commands["ca"] != 2 || rep.Multiline != 2 {
		t.Errorf("counts nicoscript=%v ca=%d multiline=%d", rep.NicoScript, rep.Commands["ca"], rep.Multiline)
	}
	if rep.Output != 2+7+1 {
		t.Errorf("output %d", rep.Output)
	}
}

func TestDeriveForks(t *testing.T) {
	out, rep, err := Derive(loadFixture(t), Options{Forks: []string{"owner"}})
	if err != nil {
		t.Fatal(err)
	}
	var threads []outThread
	_ = json.Unmarshal(out, &threads)
	if len(threads) != 1 || threads[0].Fork != "owner" || rep.Output != 2 {
		t.Fatalf("got %d threads, output %d", len(threads), rep.Output)
	}
	if rep.Threads[1].Included || rep.Threads[1].Comments != 7 {
		t.Fatalf("excluded thread should still be reported: %+v", rep.Threads[1])
	}
}

func TestLoadRejectsTampering(t *testing.T) {
	base := fstest.MapFS{}
	for _, p := range []string{"capture.json", "report.json", "responses/000001.json", "responses/000002.json", "responses/000003.json"} {
		b, err := os.ReadFile(filepath.Join("../../testdata/v1-basic", p))
		if err != nil {
			t.Fatal(err)
		}
		base[p] = &fstest.MapFile{Data: b}
	}
	if _, err := snapshot.Load(base); err != nil {
		t.Fatalf("untampered copy should load: %v", err)
	}
	clone := func() fstest.MapFS {
		m := fstest.MapFS{}
		for k, v := range base {
			m[k] = &fstest.MapFile{Data: append([]byte(nil), v.Data...)}
		}
		return m
	}

	flipped := clone()
	flipped["responses/000002.json"].Data[10] ^= 1
	extra := clone()
	extra["responses/000004.json"] = &fstest.MapFile{Data: []byte("{}")}
	missing := clone()
	delete(missing, "responses/000003.json")
	badSchema := clone()
	badSchema["capture.json"].Data = bytes.Replace(badSchema["capture.json"].Data, []byte(`"sm1"`), []byte(`"not an id"`), 1)
	for name, fsys := range map[string]fstest.MapFS{"flipped byte": flipped, "extra file": extra, "missing response": missing, "bad schema": badSchema} {
		if _, err := snapshot.Load(fsys); err == nil {
			t.Errorf("%s: Load accepted it", name)
		}
	}
}

func TestDeriveLegacyXML(t *testing.T) {
	xml, err := os.ReadFile("../../testdata/legacy.xml")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "snap")
	if _, err := importxml.Import(xml, "sm2", "legacy.xml", "test", dir); err != nil {
		t.Fatal(err)
	}
	s, err := snapshot.Load(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	if s.Report.Status != "partial" || !bytes.Equal(s.Bodies[0], xml) {
		t.Fatal("import must keep XML bytes and mark partial")
	}
	out, rep, err := Derive(s, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var threads []outThread
	if err := json.Unmarshal(out, &threads); err != nil {
		t.Fatal(err)
	}
	byFork := map[string][]json.RawMessage{}
	for _, th := range threads {
		byFork[th.Fork] = th.Comments
	}
	if len(byFork["main"]) != 3 || len(byFork["owner"]) != 2 {
		t.Fatalf("main %d owner %d", len(byFork["main"]), len(byFork["owner"]))
	}
	var first, scored map[string]any
	_ = json.Unmarshal(byFork["main"][0], &first)
	_ = json.Unmarshal(byFork["main"][2], &scored)
	if first["body"] != "A & B あ <b>" {
		t.Errorf("entities: %q", first["body"])
	}
	if strings.Join(toStrings(first["commands"]), ",") != "ue,red,big" {
		t.Errorf("full-width space split: %v", first["commands"])
	}
	if first["postedAt"] != "2026-01-01T00:00:00.25+09:00" || first["isPremium"] != true || first["vposMs"] != 0.0 {
		t.Errorf("first %v", first)
	}
	if scored["score"] != -4800.0 || scored["nicoruCount"] != 2.0 || scored["isPremium"] != false {
		t.Errorf("scored %v", scored)
	}
	if !bytes.Contains(out, []byte(`"body":"line1\nline2"`)) {
		t.Error("multiline body lost")
	}
	l := rep.Legacy
	if l.Deleted != 1 || l.ForkSources["attr"] != 1 || l.ForkSources["no-userid"] != 1 || l.ForkSources["assumed"] != 4 ||
		l.UnknownAttrs["mystery"] != 1 || l.DateUsecAllZero || l.PremiumValues["3"] != 1 {
		t.Errorf("legacy report %+v", l)
	}
	if len(rep.Invalid) != 1 || rep.Invalid[0].Reason != "missing vpos" {
		t.Errorf("invalid %+v", rep.Invalid)
	}
}

func TestDeriveZouryouJSON(t *testing.T) {
	b, err := os.ReadFile("../../testdata/zouryou.json")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "snap")
	if _, err := importzouryou.Import(b, "sm3", "zouryou.json", "test", dir); err != nil {
		t.Fatal(err)
	}
	s, err := snapshot.Load(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	out, rep, err := Derive(s, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var threads []outThread
	if err := json.Unmarshal(out, &threads); err != nil {
		t.Fatal(err)
	}
	if len(threads) != 2 || threads[0].Fork != "main" || threads[1].Fork != "owner" {
		t.Fatalf("threads %+v", threads)
	}
	// Full V1 objects pass through: score survives, the duplicate is dropped.
	if len(threads[0].Comments) != 3 || !bytes.Contains(out, []byte(`"score":-5000`)) {
		t.Errorf("main comments %d", len(threads[0].Comments))
	}
	m := rep.Threads[0]
	if m.Duplicates != 1 || m.NumberRange != [2]int64{1, 5} || m.MissingNumbers != 2 {
		t.Errorf("main report %+v", m)
	}
	if s.Report.Status != "partial" || len(s.Report.Lossy) == 0 {
		t.Error("import must be partial with lossy notes")
	}
}

func toStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}
