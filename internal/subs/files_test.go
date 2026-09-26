package subs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBeside(t *testing.T) {
	d := t.TempDir()
	for _, n := range []string{"ep 1.mkv", "ep 1.srt", "ep 1.chs.ass", "ep 1[CHT].srt", "ep 1.ja.vtt", "ep 10.srt", "ep 1.txt", "other.srt", "EP 1.en.SRT"} {
		os.WriteFile(filepath.Join(d, n), nil, 0o644)
	}
	got := Beside(filepath.Join(d, "ep 1.mkv"))
	want := map[string]string{"EP 1.en.SRT": "en", "ep 1.chs.ass": "zh-Hans", "ep 1.ja.vtt": "ja", "ep 1.srt": "", "ep 1[CHT].srt": "zh-Hant"}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for _, f := range got {
		if l, ok := want[f.Name]; !ok || l != f.Lang {
			t.Errorf("%s: lang %q (want %q, listed %v)", f.Name, f.Lang, l, ok)
		}
	}
}

func TestLangOfTag(t *testing.T) {
	for tag, want := range map[string]string{".sc": "zh-Hans", "[简日双语]": "zh-Hans", "_Big5": "zh-Hant", "jpn": "ja", ".unknown": "", "": ""} {
		if got := LangOfTag(tag); got != want {
			t.Errorf("LangOfTag(%q) = %q, want %q", tag, got, want)
		}
	}
}
