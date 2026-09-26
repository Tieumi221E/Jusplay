package safeswap

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		return "<missing>"
	}
	return string(b)
}

func TestSwap(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "a.mkv")
	write(t, p, "old")
	write(t, Temp(p), "new")
	if err := Swap(Temp(p), p); err != nil {
		t.Fatal(err)
	}
	if read(t, p) != "new" || read(t, p+OldSuffix) != "<missing>" || read(t, Temp(p)) != "<missing>" {
		t.Fatalf("after swap: %q %q %q", read(t, p), read(t, p+OldSuffix), read(t, Temp(p)))
	}
}

// Every point a swap can be cut at, and what Recover must leave.
func TestRecover(t *testing.T) {
	cases := []struct {
		name          string
		path, old, tp string // "" = absent
		want          string // content at path afterwards
	}{
		{"nothing to do", "orig", "", "", "orig"},
		{"cut while writing the new file", "orig", "", "half", "orig"},
		{"cut between the two renames", "", "orig", "new", "orig"},
		{"cut between the renames, new file lost", "", "orig", "", "orig"},
		{"cut before removing the old copy", "new", "orig", "", "new"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := t.TempDir()
			p := filepath.Join(d, "a.mkv")
			for f, s := range map[string]string{p: c.path, p + OldSuffix: c.old, Temp(p): c.tp} {
				if s != "" {
					write(t, f, s)
				}
			}
			if _, err := Recover(p); err != nil {
				t.Fatal(err)
			}
			if got := read(t, p); got != c.want {
				t.Fatalf("path holds %q, want %q", got, c.want)
			}
			if read(t, p+OldSuffix) != "<missing>" || read(t, Temp(p)) != "<missing>" {
				t.Fatalf("leftovers remain: old %q temp %q", read(t, p+OldSuffix), read(t, Temp(p)))
			}
		})
	}
}

// A guard that fails is only worth something if it fails when it should:
// a swap over a leftover old copy must refuse, not overwrite it.
func TestSwapRefusesOverLeftover(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "a.mkv")
	write(t, p, "cur")
	write(t, p+OldSuffix, "keep")
	write(t, Temp(p), "new")
	if err := Swap(Temp(p), p); err == nil {
		t.Fatal("swap over a leftover old copy succeeded")
	}
	if read(t, p+OldSuffix) != "keep" || read(t, p) != "cur" {
		t.Fatal("files changed by a refused swap")
	}
}
