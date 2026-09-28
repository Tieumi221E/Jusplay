package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tieumi221E/Jusplay/internal/library"
)

func makeSkill(t *testing.T, folder, name string) string {
	t.Helper()
	exe, _ := os.Executable()
	dir := filepath.Join(folder, library.VaultDir, "skills", name)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: says where it ran\n---\n"), 0o644)
	spec, _ := json.Marshal(map[string]any{"run": []string{exe}})
	os.WriteFile(filepath.Join(dir, "skill.json"), spec, 0o644)
	return dir
}

// A media folder's skill: listed, refused until confirmed, run in the
// folder with the series' variables, confirmed again after a change.
func TestSkills(t *testing.T) {
	t.Setenv("JUSPLAY_TEST_SKILL", "1")
	data, media := fixture(t)
	run(t, data, "library", "add", media)
	dir := makeSkill(t, media, "where")

	_, out, _ := run(t, data, "skills", "list", "-json")
	var list []struct {
		Name    string `json:"name"`
		Folder  string `json:"folder"`
		Trusted bool   `json:"trusted"`
	}
	json.Unmarshal([]byte(out), &list)
	if len(list) != 1 || list[0].Name != "where" || list[0].Folder != media || list[0].Trusted {
		t.Fatalf("skills list: %s", out)
	}
	if code, _, e := run(t, data, "skills", "run", "where", "-json"); code != 2 || !strings.Contains(e, `"kind":"confirm"`) {
		t.Fatalf("unconfirmed: exit %d %s", code, e)
	}
	if code, _, e := run(t, data, "skills", "run", "where", "-yes"); code != 0 {
		t.Fatalf("confirmed run: exit %d %s", code, e)
	}
	got, _ := os.ReadFile(filepath.Join(media, library.VaultDir, "out", "where", "where.txt"))
	if string(got) != media+"|skill/where" {
		t.Fatalf("where it ran: %q", got)
	}
	if code, _, e := run(t, data, "skills", "run", "where"); code != 0 {
		t.Fatalf("second run needs no -yes: exit %d %s", code, e)
	}
	os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("changed"), 0o644)
	if code, _, _ := run(t, data, "skills", "run", "where"); code != 2 {
		t.Fatalf("a changed skill ran without being confirmed again: exit %d", code)
	}

	// The same name in two folders: say which.
	other := filepath.Join(t.TempDir(), "other")
	os.MkdirAll(other, 0o755)
	run(t, data, "library", "add", other)
	makeSkill(t, other, "where")
	if code, _, e := run(t, data, "skills", "run", "where", "-yes"); code != 2 || !strings.Contains(e, "-folder") {
		t.Fatalf("ambiguous: exit %d %s", code, e)
	}
	if code, _, e := run(t, data, "skills", "run", "where", "-folder", other, "-yes"); code != 0 {
		t.Fatalf("with -folder: exit %d %s", code, e)
	}
}
