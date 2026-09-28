package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain lets this test binary also play Jusnote for note add (the
// variable names where to record the call): the apps call each other
// through their command lines, and this one is the other side.
func TestMain(m *testing.M) {
	// Also a skill's program (skills_test.go): it says where it ran.
	if os.Getenv("JUSPLAY_TEST_SKILL") == "1" {
		out := os.Getenv("JUS_OUT")
		os.WriteFile(filepath.Join(out, "where.txt"), []byte(os.Getenv("JUSPLAY_FOLDER")+"|"+os.Getenv("JUS_HARNESS")), 0o644)
		os.Stdout.WriteString("done\n")
		os.Exit(0)
	}
	if rec := os.Getenv("JUSPLAY_TEST_FAKE_JUSNOTE"); rec != "" {
		args := os.Args[1:]
		var text []byte
		for i, a := range args {
			if a == "-file" && i+1 < len(args) {
				text, _ = os.ReadFile(args[i+1])
			}
		}
		b, _ := json.Marshal(map[string]any{"args": args, "text": string(text)})
		os.WriteFile(rec, b, 0o644)
		os.Stdout.WriteString(`{"committed":true}` + "\n")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestNoteAdd(t *testing.T) {
	data, media := fixture(t)
	run(t, data, "library", "add", media)
	rec := filepath.Join(t.TempDir(), "call.json")
	t.Setenv("JUSPLAY_TEST_FAKE_JUSNOTE", "")
	ep := filepath.Join(media, "Show", "Show - 02.mkv")
	nb := t.TempDir()

	// Jusnote not here: said so.
	t.Setenv("JUS_JUSNOTE_EXE", filepath.Join(t.TempDir(), "none.exe"))
	t.Setenv("PATH", "")
	if code, _, e := run(t, data, "note", "add", "-entry", ep, "-text", "x", "-notebook", nb); code != 1 || !strings.Contains(e, "not installed") {
		t.Fatalf("no Jusnote: exit %d %s", code, e)
	}
	// Nothing playing and no -entry: a usage error, not a guess.
	if code, _, e := run(t, data, "note", "add", "-text", "x", "-notebook", nb); code != 2 || !strings.Contains(e, "no video is playing") {
		t.Fatalf("no video: exit %d %s", code, e)
	}

	// An agent's note: through Jusnote's append, with its name, left for review.
	exe, _ := os.Executable()
	t.Setenv("JUS_JUSNOTE_EXE", exe)
	t.Setenv("JUSPLAY_TEST_FAKE_JUSNOTE", rec)
	code, out, e := run(t, data, "note", "add", "-entry", ep, "-at", "83.5", "-text", "a  good\nscene", "-notebook", nb,
		"-author", "agent", "-harness", "codex", "-run", "s1", "-json")
	if code != 0 {
		t.Fatalf("note add: exit %d %s", code, e)
	}
	var call struct {
		Args []string
		Text string
	}
	b, _ := os.ReadFile(rec)
	json.Unmarshal(b, &call)
	joined := strings.Join(call.Args, " ")
	for _, want := range []string{"append Jusplay.md", "-notebook " + nb, "-author agent", "-harness codex", "-run s1", "-no-commit", "-json"} {
		if !strings.Contains(joined, want) {
			t.Errorf("jusnote called without %q: %s", want, joined)
		}
	}
	if !strings.Contains(call.Text, "[Show - 02 1:23](jus://play/") || !strings.HasSuffix(strings.TrimSpace(call.Text), "— a good scene") {
		t.Errorf("the line: %q", call.Text)
	}
	if m := decode(t, out); m["file"] != "Jusplay.md" || !strings.HasPrefix(m["link"].(string), "jus://play/") {
		t.Errorf("result: %v", m)
	}
}
