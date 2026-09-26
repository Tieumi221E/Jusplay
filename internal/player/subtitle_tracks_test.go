package player

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

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
	get := func(path string) (int, string) {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", "/"+s.token+"/"+path, nil))
		return w.Code, w.Body.String()
	}
	post := func(path, body string) (int, string) {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("POST", "/"+s.token+"/"+path, strings.NewReader(body)))
		return w.Code, w.Body.String()
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

	// A file beside the video is served; nothing else, whatever the key says.
	if code, body := get("api/subs/file?id=" + e.ID + "&key=file:S%20-%2001.chs.srt"); code != 200 || body != string(srt) {
		t.Errorf("beside: %d %q", code, body)
	}
	for _, key := range []string{"file:secret.txt", "file:..%2Fsecret.txt", "file:" + strings.ReplaceAll(other, " ", "%20"), "manual"} {
		if code, _ := get("api/subs/file?id=" + e.ID + "&key=" + key); code != 404 {
			t.Errorf("key %s: %d, want 404", key, code)
		}
	}

	if code, body := get("api/subs/embedded?id=" + e.ID + "&track=" + jsonNum(tr.Embedded[1].Number)); code != 200 || !strings.Contains(body, `\N二行目`) || !strings.Contains(body, "[V4+ Styles]") {
		t.Errorf("embedded: %d %s", code, body)
	}

	// Picking: only subtitle files; the choice is kept with the video.
	if code, _ := post("api/subs/pick", `{"id":"`+e.ID+`","pick":"manual","file":`+strconvQuote(filepath.Join(dir, "secret.txt"))+`}`); code != 422 {
		t.Errorf("picked a .txt: %d", code)
	}
	if code, body := post("api/subs/pick", `{"id":"`+e.ID+`","pick":"manual","pick2":"mkv:4","file":`+strconvQuote(other)+`}`); code != 204 {
		t.Fatalf("pick: %d %s", code, body)
	}
	if x, _ := lib.Get(e.ID); x.SubPick != "manual" || x.SubPick2 != "mkv:4" || x.SubFile != other {
		t.Errorf("kept %q %q %q", x.SubPick, x.SubPick2, x.SubFile)
	}
	if code, body := get("api/subs/file?id=" + e.ID + "&key=manual"); code != 200 || body != string(ass) {
		t.Errorf("manual: %d", code)
	}
}

func jsonNum(n uint64) string { b, _ := json.Marshal(n); return string(b) }
