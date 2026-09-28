package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The commands that took -ffmpeg and -ffprobe before Jusplay read media
// itself still accept them (and ignore them): scripts written against them
// must not start failing on an unknown flag.
func TestLegacyToolFlagsAccepted(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "jusplay.exe")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	missing := filepath.Join(t.TempDir(), "missing.mkv")
	for _, cmd := range []string{"embed", "pack", "attach", "verify"} {
		out, _ := exec.Command(bin, cmd, "-ffmpeg", "ffmpeg.exe", "-ffprobe", "ffprobe.exe", missing).CombinedOutput()
		if strings.Contains(string(out), "flag provided but not defined") {
			t.Errorf("%s refuses -ffmpeg/-ffprobe:\n%s", cmd, out)
		}
	}
}
