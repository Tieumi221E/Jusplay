package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tieumi221E/Jus/capreg"
	"github.com/Tieumi221E/Jusplay/internal/library"
)

// fixture is a data folder and a media folder with three fake episodes
// (the scanner only looks at names; probing a fake file just fails).
func fixture(t *testing.T) (data, media string) {
	t.Helper()
	root := t.TempDir()
	data, media = filepath.Join(root, "data"), filepath.Join(root, "media")
	os.MkdirAll(filepath.Join(media, "Show"), 0o755)
	for _, n := range []string{"01", "02", "03"} {
		os.WriteFile(filepath.Join(media, "Show", "Show - "+n+".mkv"), []byte("x"), 0o644)
	}
	return data, media
}

// run is `jusplay -data <data> <args…>`: the exit code and stdout.
func run(t *testing.T, data string, args ...string) (int, string, string) {
	t.Helper()
	reg := newRegistry(nil)
	c, rest, ok := reg.Resolve(args)
	if !ok {
		t.Fatalf("no command %v", args)
	}
	var out, errb bytes.Buffer
	code := runCap(c, rest, data, &out, &errb)
	return code, out.String(), errb.String()
}

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, s)
	}
	return m
}

func TestCommandLineHere(t *testing.T) {
	data, media := fixture(t)
	ep2 := filepath.Join(media, "Show", "Show - 02.mkv")

	if code, out, e := run(t, data, "library", "add", media, "-json"); code != 0 || decode(t, out)["added"] != 3.0 {
		t.Fatalf("library add: %d %s %s", code, out, e)
	}
	code, out, _ := run(t, data, "library", "list", "-json")
	if code != 0 || len(decode(t, out)["entries"].([]any)) != 3 {
		t.Fatalf("library list: %d %s", code, out)
	}

	code, out, _ = run(t, data, "progress", "set", ep2, "-position", "1:30", "-json")
	if code != 0 || decode(t, out)["position"] != 90.0 {
		t.Fatalf("progress set: %d %s", code, out)
	}
	// The change is in the folder's records, where the window reads it.
	l, err := library.Open(filepath.Join(data, "folders.json"))
	if err != nil {
		t.Fatal(err)
	}
	if e, _ := l.Get(library.IDFor(ep2)); e.Position != 90 {
		t.Fatalf("records say %v", e.Position)
	}

	// A change says whether it was kept: under an added folder it is; a
	// file outside every folder has its records in memory only.
	if m := decode(t, out); m["saved"] != true {
		t.Fatalf("saved under a folder: %v", m)
	}
	loose := filepath.Join(t.TempDir(), "Loose - 01.mkv")
	os.WriteFile(loose, []byte("x"), 0o644)
	_, out, _ = run(t, data, "progress", "set", loose, "-position", "5", "-json")
	if m := decode(t, out); m["saved"] != false || m["note"] == nil {
		t.Fatalf("saved outside the folders: %v", m)
	}

	// A write based on an old version is refused with exit 3.
	_, out, _ = run(t, data, "progress", "get", ep2, "-json")
	v := decode(t, out)["version"].(string)
	run(t, data, "entry", "watched", ep2)
	if code, _, out := run(t, data, "progress", "set", ep2, "-position", "3", "-base", v, "-json"); code != capreg.ExitConflict || decode(t, out)["kind"] != "conflict" || decode(t, out)["code"] != 3.0 {
		t.Fatalf("stale base: %d %s", code, out)
	}

	// Usage errors exit 2; unknown videos 1; confirm commands want -yes.
	if code, _, _ := run(t, data, "progress", "set", ep2); code != capreg.ExitUsage {
		t.Fatalf("missing -position: %d", code)
	}
	if code, _, _ := run(t, data, "progress", "get", "no-such-video"); code != capreg.ExitFail {
		t.Fatalf("unknown video: %d", code)
	}
	code, _, out = run(t, data, "comments", "embed", ep2, "-json") // a failure's JSON is on stderr
	if m := decode(t, out); code != capreg.ExitUsage || m["kind"] != "confirm" || m["plan"] == nil {
		t.Fatalf("embed without -yes: %d %s", code, out)
	}

	// Settings: a merge patch on the page's document.
	run(t, data, "settings", "set", "-patch", `{"comments":{"opacity":0.5,"size":"big"}}`)
	run(t, data, "settings", "set", "-patch", `{"comments":{"size":null}}`)
	_, out, _ = run(t, data, "settings", "get", "-json")
	if c := decode(t, out)["comments"].(map[string]any); c["opacity"] != 0.5 || c["size"] != nil {
		t.Fatalf("settings: %s", out)
	}
	if code, _, _ := run(t, data, "prefs", "set", "-theme", "purple"); code != capreg.ExitUsage {
		t.Fatalf("bad pref accepted: %d", code)
	}
}

// With a window open, calls run in it: the same results, and the window's
// own library (not just the files) has the change.
func TestCommandLineHandsCallsToTheWindow(t *testing.T) {
	data, media := fixture(t)
	env, err := openApp(data, false, false)
	if err != nil {
		t.Fatal(err)
	}
	base, err := env.srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer env.srv.Close()
	defer announce(env.dir, base)()
	if _, ok := findInstance(data); !ok {
		t.Fatal("the running window is not found")
	}

	if code, out, e := run(t, data, "library", "add", media, "-json"); code != 0 {
		t.Fatalf("library add via window: %d %s %s", code, out, e)
	}
	ep1 := filepath.Join(media, "Show", "Show - 01.mkv")
	if code, out, _ := run(t, data, "progress", "set", ep1, "-position", "42", "-json"); code != 0 || decode(t, out)["position"] != 42.0 {
		t.Fatalf("progress via window: %d %s", code, out)
	}
	if e, _ := env.lib.Get(library.IDFor(ep1)); e.Position != 42 {
		t.Fatalf("the window's library has %v", e.Position)
	}
	// Errors keep their exit codes across the hand-over.
	if code, _, _ := run(t, data, "progress", "set", ep1); code != capreg.ExitUsage {
		t.Fatalf("usage via window: %d", code)
	}
	if code, _, _ := run(t, data, "progress", "get", "nope"); code != capreg.ExitFail {
		t.Fatalf("not found via window: %d", code)
	}
	// Human output is the same either way.
	_, viaWindow, _ := run(t, data, "library", "list")
	if !strings.Contains(viaWindow, "Show - 01.mkv") || !strings.Contains(viaWindow, "3 files") {
		t.Fatalf("list via window:\n%s", viaWindow)
	}
}

func TestHelpDescribesEveryCommand(t *testing.T) {
	var b bytes.Buffer
	cmdHelp([]string{"-json"}, &b)
	var h capreg.HelpDoc
	if err := json.Unmarshal(b.Bytes(), &h); err != nil {
		t.Fatal(err)
	}
	reg := newRegistry(nil)
	if len(h.Commands) != len(reg.All()) || len(h.Commands) < 20 {
		t.Fatalf("help lists %d of %d", len(h.Commands), len(reg.All()))
	}
	for _, c := range h.Commands {
		if c.Summary == "" || c.Synopsis == "" {
			t.Errorf("%s: no summary or synopsis", c.ID)
		}
		for _, p := range c.Params {
			if p.Doc == "" {
				t.Errorf("%s -%s: no doc", c.ID, p.Name)
			}
		}
	}
	b.Reset()
	cmdHelp(nil, &b)
	if !strings.Contains(b.String(), "progress set <entry> -position number") || !strings.Contains(b.String(), "import-xml") {
		t.Fatalf("usage:\n%s", b.String())
	}
}

// A fake page on the window's event stream: it carries out commands the
// way the player does and reports its state; `player seek` from the command
// line returns the state the page reported.
func TestLiveSession(t *testing.T) {
	data, media := fixture(t)
	env, err := openApp(data, false, false)
	if err != nil {
		t.Fatal(err)
	}
	base, err := env.srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer env.srv.Close()
	defer announce(env.dir, base)()
	run(t, data, "library", "add", media)
	ep1 := filepath.Join(media, "Show", "Show - 01.mkv")

	// Without a page listening, commands say so (and do not hang).
	if code, _, out := run(t, data, "player", "pause", "-json"); code != capreg.ExitFail || !strings.Contains(out, "no window page") {
		t.Fatalf("pause with no page: %d %s", code, out)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	page := capreg.Remote{Base: base, Source: capreg.Source{Via: "window"}}
	events := make(chan map[string]any, 64)
	post := func(body map[string]any) {
		b, _ := json.Marshal(body)
		res, err := http.Post(base+"api/state", "application/json", bytes.NewReader(b))
		if err == nil {
			res.Body.Close()
		}
	}
	go page.Stream(ctx, "events.watch", nil, func(v any) error {
		ev := v.(map[string]any)
		events <- ev
		if ev["kind"] != "command" {
			return nil
		}
		d := ev["data"].(map[string]any)
		st := map[string]any{"page": "player", "id": library.IDFor(ep1), "paused": false}
		switch d["cmd"] {
		case "seek":
			to, _ := d["to"].(json.Number).Float64()
			st["position"] = to
		case "pause":
			st["paused"] = true
		}
		go post(map[string]any{"state": st, "ack": d["id"]})
		return nil
	})
	waitFor := func(kind string) map[string]any {
		t.Helper()
		for {
			select {
			case ev := <-events:
				if ev["kind"] == kind {
					return ev
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("no %s event", kind)
			}
		}
	}
	waitFor("hello")

	t0 := time.Now()
	code, out, e := run(t, data, "player", "seek", "-to", "2:05", "-json")
	if code != 0 || decode(t, out)["position"] != 125.0 {
		t.Fatalf("seek: %d %s %s", code, out, e)
	}
	t.Logf("player seek round trip (command line → window → page → answer): %s", time.Since(t0).Round(time.Millisecond))
	if st := waitFor("command")["data"].(map[string]any); st["cmd"] != "seek" {
		t.Fatalf("command event %v", st)
	}
	_, out, _ = run(t, data, "player", "status", "-json")
	if s := decode(t, out)["state"].(map[string]any); s["position"] != 125.0 || s["page"] != "player" {
		t.Fatalf("status %s", out)
	}
	if code, _, _ := run(t, data, "player", "seek", "-to", "1", "-by", "2"); code != capreg.ExitUsage {
		t.Fatalf("seek -to and -by: %d", code)
	}

	// A change from the command line is an event, with who made it.
	t.Setenv("JUS_HARNESS", "test-harness")
	run(t, data, "progress", "set", ep1, "-position", "30", "-run", "session-7")
	ev := waitFor("changed")
	src := ev["source"].(map[string]any)
	if ev["data"].(map[string]any)["cap"] != "progress.set" || src["harness"] != "test-harness" || src["run"] != "session-7" || src["via"] != "cli" {
		t.Fatalf("changed event %v", ev)
	}
}

// Two agents' sessions and a person change things; one session is undone:
// its changes go back, the others stay, and what someone else changed
// since is reported, not overwritten.
func TestRevertASession(t *testing.T) {
	data, media := fixture(t)
	run(t, data, "library", "add", media)
	ep := func(n string) string { return filepath.Join(media, "Show", "Show - "+n+".mkv") }
	as := func(session string, args ...string) (int, string) {
		t.Helper()
		t.Setenv("JUS_HARNESS", "test-harness")
		t.Setenv("JUS_RUN", session)
		code, out, e := run(t, data, append(args, "-json")...)
		if code != 0 {
			return code, e // a failure's JSON is on stderr
		}
		return code, out
	}
	as("A", "progress", "set", ep("01"), "-position", "30")
	as("A", "entry", "watched", ep("02"))
	as("A", "settings", "set", "-patch", `{"comments":{"opacity":0.3}}`)
	as("B", "progress", "set", ep("03"), "-position", "70")
	as("A", "progress", "set", ep("03"), "-position", "10") // after B: A's own, on top of B's
	as("", "progress", "set", ep("01"), "-position", "55")  // a person, later: A's change to 01 is now theirs

	_, out := as("", "changes", "list", "-session", "A")
	var listed []map[string]any
	json.Unmarshal([]byte(out), &listed)
	if len(listed) != 4 || listed[0]["source"].(map[string]any)["harness"] != "test-harness" {
		t.Fatalf("changes list -run A: %s", out)
	}

	// The plan first (no -yes), then for real.
	code, out := as("", "changes", "revert", "-session", "A")
	if code != capreg.ExitUsage || !strings.Contains(out, `"plan"`) {
		t.Fatalf("revert without -yes: %d %s", code, out)
	}
	code, out = as("", "changes", "revert", "-session", "A", "-yes")
	if code != 0 {
		t.Fatalf("revert: %d %s", code, out)
	}
	var steps []map[string]any
	json.Unmarshal([]byte(out), &steps)
	states := map[string]string{}
	for _, s := range steps {
		name := "-"
		if p, ok := s["path"].(string); ok {
			name = filepath.Base(p)
		}
		states[fmt.Sprint(s["cap"], " ", name)] = fmt.Sprint(s["state"])
	}
	want := map[string]string{
		"progress.set Show - 01.mkv":  "conflict", // the person changed it since
		"entry.watched Show - 02.mkv": "done",
		"settings.set -":              "done",
		"progress.set Show - 03.mkv":  "done", // back to B's 70
	}
	for k, v := range want {
		if states[k] != v {
			t.Errorf("%s: %s, want %s (all: %v)", k, states[k], v, states)
		}
	}
	pos := func(n string) float64 {
		_, out := as("", "progress", "get", ep(n))
		return decode(t, out)["position"].(float64)
	}
	if p := pos("01"); p != 55 {
		t.Errorf("01: %v, the person's 55 must stay", p)
	}
	if p := pos("03"); p != 70 {
		t.Errorf("03: %v, back to B's 70", p)
	}
	_, out = as("", "progress", "get", ep("02"))
	if decode(t, out)["watched"] != false {
		t.Errorf("02 still watched")
	}
	_, out = as("", "settings", "get")
	if c := decode(t, out)["comments"]; c != nil && c.(map[string]any)["opacity"] == 0.3 {
		t.Errorf("settings not put back: %s", out)
	}
	// The logs: the video's in its folder, the app's in the data folder.
	if _, err := os.Stat(filepath.Join(media, library.VaultDir, "changes.jsonl")); err != nil {
		t.Error("no change log in the media folder")
	}
	if _, err := os.Stat(filepath.Join(data, "changes.jsonl")); err != nil {
		t.Error("no change log in the data folder")
	}
}

// A link to a moment still opens the video after the folder moved (another
// drive, another computer): it names the folder by its own id.
func TestLinksSurviveAMovedFolder(t *testing.T) {
	data, media := fixture(t)
	run(t, data, "library", "add", media)
	ep2 := filepath.Join(media, "Show", "Show - 02.mkv")
	_, out, _ := run(t, data, "link", "make", ep2, "-at", "1:23.5", "-json")
	m := decode(t, out)
	link := m["link"].(string)
	if !strings.HasPrefix(link, "jus://play/") || !strings.HasSuffix(link, "/Show/Show%20-%2002.mkv?t=83.5") || m["portable"] != true {
		t.Fatalf("link %v", m)
	}
	if md := m["markdown"].(string); !strings.Contains(md, "1:23](jus://play/") {
		t.Fatalf("markdown %q", md)
	}
	// Move the folder, add it at its new place: the same link works.
	moved := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.Rename(media, moved); err != nil {
		t.Fatal(err)
	}
	run(t, data, "library", "remove", media)
	run(t, data, "library", "add", moved)
	code, out, e := run(t, data, "progress", "get", link, "-json")
	if code != 0 || decode(t, out)["path"] != filepath.Join(moved, "Show", "Show - 02.mkv") {
		t.Fatalf("after the move: %d %s %s", code, out, e)
	}
	// A link to a folder not in the library says so.
	run(t, data, "library", "remove", moved)
	if code, _, _ := run(t, data, "progress", "get", link); code != capreg.ExitFail {
		t.Fatalf("unknown folder: %d", code)
	}
}

func TestAgentsGuideListsEveryCommand(t *testing.T) {
	reg := newRegistry(nil)
	_, out, _ := run(t, t.TempDir(), "agents")
	for _, c := range reg.All() {
		if !strings.Contains(out, "`jusplay "+c.Synopsis()+"`") {
			t.Errorf("the guide lacks %s", c.ID)
		}
	}
	// Written only when asked, and never over someone else's AGENTS.md.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# mine\n"), 0o644)
	if code, _, _ := run(t, t.TempDir(), "agents", "-write", dir); code != capreg.ExitFail {
		t.Fatalf("overwrote another AGENTS.md: %d", code)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); string(b) != "# mine\n" {
		t.Fatal("changed another AGENTS.md")
	}
}
