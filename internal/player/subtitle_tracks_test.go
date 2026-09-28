package player

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Tieumi221E/Jus/capreg"
	"github.com/Tieumi221E/Jusplay/internal/library"
)

func TestSubtitleEndpoints(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "S - 01.mkv")
	b, err := os.ReadFile("../../testdata/media/subs.mkv")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(video, b, 0o644)
	srt, _ := os.ReadFile("../../testdata/subs/tiny.srt")
	os.WriteFile(filepath.Join(dir, "S - 01.chs.srt"), srt, 0o644)
	os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("not for the page"), 0o644)
	other := filepath.Join(t.TempDir(), "picked.ass")
	ass, _ := os.ReadFile("../../testdata/subs/tiny.ass")
	os.WriteFile(other, ass, 0o644)

	lib, err := library.Open(filepath.Join(dir, "folders.json"))
	if err != nil {
		t.Fatal(err)
	}
	e, err := lib.Ensure(video)
	if err != nil {
		t.Fatal(err)
	}
	s := New(lib, fstest.MapFS{}, OpenSettings(""), "", t.TempDir())
	s.Caps = capreg.New("test", "0")
	Register(s.Caps, func() (*Server, error) { return s, nil })
	get := func(path string) (int, string) {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", "/"+s.token+"/"+path, nil))
		return w.Code, w.Body.String()
	}
	post := func(path, body string) (int, string) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/"+s.token+"/"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		s.ServeHTTP(w, r)
		return w.Code, w.Body.String()
	}
	// subs.show, as the page calls it: the lines' texts, or the error status.
	show := func(key string) (int, string) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/"+s.token+"/api/cap/subs.show", strings.NewReader(`{"entry":"`+e.ID+`","track":`+strconvQuote(key)+`}`))
		r.Header.Set("Content-Type", "application/json")
		s.ServeHTTP(w, r)
		var out struct{ Lines []struct{ Text string } }
		json.Unmarshal(w.Body.Bytes(), &out)
		var texts []string
		for _, l := range out.Lines {
			texts = append(texts, l.Text)
		}
		return w.Code, strings.Join(texts, "|")
	}

	code, body := get("api/subs/tracks?id=" + e.ID)
	var tr struct {
		Files    []struct{ Name, Lang string }
		Embedded []struct {
			Number uint64
			Codec  string
			Lang   string
		}
		AI struct{ Available bool }
	}
	if err := json.Unmarshal([]byte(body), &tr); code != 200 || err != nil {
		t.Fatalf("tracks: %d %s", code, body)
	}
	if len(tr.Files) != 1 || tr.Files[0].Name != "S - 01.chs.srt" || tr.Files[0].Lang != "zh-Hans" {
		t.Errorf("files %+v", tr.Files)
	}
	if len(tr.Embedded) != 2 || tr.Embedded[0].Lang != "zh-Hans" || tr.Embedded[1].Codec != "S_TEXT/ASS" || tr.Embedded[1].Lang != "ja" {
		t.Errorf("embedded %+v", tr.Embedded)
	}
	if tr.AI.Available {
		t.Error("AI available without an engine")
	}

	// A file beside the video is read; nothing else, whatever the key says.
	if code, lines := show("file:S - 01.chs.srt"); code != 200 || lines != "第一行字幕\n斜体|Second line & more" {
		t.Errorf("beside: %d %q", code, lines)
	}
	for _, key := range []string{"file:secret.txt", "file:../secret.txt", "file:" + other, "manual"} {
		if code, _ := show(key); code != 404 {
			t.Errorf("key %s: %d, want 404", key, code)
		}
	}

	// The file's own ASS track, read as the page shows it.
	if code, lines := show("mkv:" + jsonNum(tr.Embedded[1].Number)); code != 200 || !strings.Contains(lines, "\n二行目") {
		t.Errorf("embedded: %d %q", code, lines)
	}

	// Picking: only subtitle files; the choice is kept with the video.
	if code, _ := post("api/cap/subs.pick", `{"entry":"`+e.ID+`","pick":"manual","file":`+strconvQuote(filepath.Join(dir, "secret.txt"))+`}`); code != 400 {
		t.Errorf("picked a .txt: %d", code)
	}
	if code, body := post("api/cap/subs.pick", `{"entry":"`+e.ID+`","pick":"manual","pick2":"mkv:4","file":`+strconvQuote(other)+`}`); code != 200 {
		t.Fatalf("pick: %d %s", code, body)
	}
	if x, _ := lib.Get(e.ID); x.SubPick != "manual" || x.SubPick2 != "mkv:4" || x.SubFile != other {
		t.Errorf("kept %q %q %q", x.SubPick, x.SubPick2, x.SubFile)
	}
	if code, lines := show("manual"); code != 200 || lines != "こんにちは\n二行目|上に出る行" {
		t.Errorf("manual: %d %q", code, lines)
	}
}

func jsonNum(n uint64) string { b, _ := json.Marshal(n); return string(b) }
