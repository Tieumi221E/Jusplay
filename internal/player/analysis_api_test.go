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

func TestAnalysisCache(t *testing.T) {
	dir, temp := t.TempDir(), t.TempDir()
	video := filepath.Join(dir, "S - 01.mkv")
	b, _ := os.ReadFile("../../testdata/media/tiny.mkv")
	os.WriteFile(video, b, 0o644)
	src, _ := os.ReadFile("../../testdata/zouryou.json")
	os.WriteFile(filepath.Join(dir, "S - 01.json"), src, 0o644)
	lib, _ := library.Open(filepath.Join(dir, "folders.json"))
	e, err := lib.Ensure(video)
	if err != nil {
		t.Fatal(err)
	}
	get := func() string {
		s := New(lib, fstest.MapFS{}, OpenSettings(""), "", temp)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", "/"+s.token+"/api/analysis?id="+e.ID, nil))
		if w.Code != 200 {
			t.Fatalf("analysis: %d %s", w.Code, w.Body)
		}
		return w.Body.String()
	}
	first := get()
	if !strings.Contains(first, `"comments":`) {
		t.Fatalf("report %.200s", first)
	}
	// A marked report under the same key comes back as it is: it was read.
	path := filepath.Join(temp, "analysis", e.ID+".json")
	var f analysisFile
	raw, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, &f) != nil {
		t.Fatalf("cache file: %v", err)
	}
	f.Report = json.RawMessage(`{"marker":true}`)
	raw, _ = json.Marshal(f)
	os.WriteFile(path, raw, 0o644)
	if got := get(); got != `{"marker":true}` {
		t.Errorf("cache not used: %.100s", got)
	}
	// Other comment data, another key: analysed again.
	os.WriteFile(filepath.Join(dir, "S - 01.json"), []byte(strings.Replace(string(src), `"一"`, `"二"`, 1)), 0o644)
	if got := get(); got == `{"marker":true}` {
		t.Error("stale cache used after the comments changed")
	}
}
