package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tieumi221E/Jus/capreg"
	"github.com/Tieumi221E/Jus/conform"
)

// The Jus conformance test (the Jus module's conform), on a build laid out
// as a release is: jusplay.exe with jusplay-data beside it.
func TestConformance(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Jusplay-test")
	bin := filepath.Join(dir, "jusplay.exe")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	rep := conform.Run(context.Background(), bin, t.TempDir(), []string{"JUSPLAY_DATA="})
	for _, c := range rep.Checks {
		if !c.OK {
			t.Errorf("%s: %s", c.Name, c.Detail)
		}
	}
	if len(rep.Checks) < 10 {
		t.Fatalf("only %d checks ran", len(rep.Checks))
	}
}

// link info: what a jus://play link points to, for a note's preview.
func TestLinkInfo(t *testing.T) {
	data, media := fixture(t)
	run(t, data, "library", "add", media)
	_, out, _ := run(t, data, "link", "make", filepath.Join(media, "Show", "Show - 02.mkv"), "-at", "83.5", "-json")
	link := decode(t, out)["link"].(string)
	code, out, e := run(t, data, "link", "info", link, "-json")
	m := decode(t, out)
	if code != 0 || !strings.Contains(m["title"].(string), "Show") || m["at"] != 83.5 || m["watched"] != false {
		t.Fatalf("link info: %d %s %s", code, out, e)
	}
	if code, _, _ := run(t, data, "link", "info", "jus://note/a.md"); code != capreg.ExitUsage {
		t.Fatalf("another app's link: exit %d, want 2", code)
	}
}
