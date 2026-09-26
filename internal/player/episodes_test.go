package player

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"testing/fstest"

	"github.com/Tieumi221E/Jusplay/internal/library"
)

// The episode list holds the series of the asked entry only (same name in
// the same added folder), in library order, without missing files.
func TestEpisodes(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	os.MkdirAll(filepath.Join(a, library.VaultDir), 0o755)
	os.MkdirAll(filepath.Join(b, library.VaultDir), 0o755)
	// Records as the app writes them: paths relative to the media folder.
	os.WriteFile(filepath.Join(a, library.VaultDir, "library.json"), []byte(`{"version":2,"entries":[
	 {"path":"S - 02.mkv","series":"S","episode":2,"title":"S - 02","size":2,"modTime":"2026-01-01T00:00:00.5Z","probe":{"duration":1400},"position":30},
	 {"path":"S - 01.mkv","series":"S","episode":1,"title":"S - 01","size":1,"modTime":"2026-01-01T00:00:00Z","watched":true},
	 {"path":"enc/S - 02.mkv","series":"S","episode":2,"title":"S - 02","size":3,"modTime":"2026-01-01T00:00:00Z","probe":{"duration":1400,"attached":true}},
	 {"path":"S - 03.mkv","series":"S","episode":3,"title":"S - 03","missing":true}]}`), 0o644)
	os.WriteFile(filepath.Join(b, library.VaultDir, "library.json"), []byte(`{"version":2,"entries":[
	 {"path":"S - 01.mkv","series":"S","episode":1,"title":"S - 01"}]}`), 0o644)
	folders := filepath.Join(dir, "folders.json")
	os.WriteFile(folders, []byte(`{"version":2,"folders":[`+strconvQuote(a)+`,`+strconvQuote(b)+`]}`), 0o644)
	lib, err := library.Open(folders)
	if err != nil {
		t.Fatal(err)
	}
	id := func(rel string) string { return library.IDFor(filepath.Join(a, rel)) }
	e1, e2, e2b := id("S - 01.mkv"), id("S - 02.mkv"), id("enc/S - 02.mkv")
	s := New(lib, fstest.MapFS{}, OpenSettings(""), "", "")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/"+s.token+"/api/episodes?id="+e2, nil))
	var got struct {
		Series   string    `json:"series"`
		Episodes []Episode `json:"episodes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, w.Body)
	}
	// Two files of episode 2: the one with comments stands for it, the
	// other is listed on it; asked for by the other one, the list is the same.
	if got.Series != "S" || len(got.Episodes) != 2 || got.Episodes[0].ID != e1 || got.Episodes[1].ID != e2b {
		t.Fatalf("episodes: %+v", got)
	}
	if e := got.Episodes[1]; e.Duration != 1400 || len(e.Versions) != 1 || e.Versions[0] != e2 || e.Thumb != "api/thumb?id="+e2b+"&v=3-1767225600000" {
		t.Errorf("row: %+v", e)
	}
	if p, n := s.neighbours(mustGet(t, lib, e2)); p != e1 || n != "" {
		t.Errorf("neighbours of the other version: %q %q", p, n)
	}
	if !got.Episodes[0].Watched {
		t.Errorf("watched lost: %+v", got.Episodes[0])
	}
	w = httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/"+s.token+"/api/episodes?id=nope", nil))
	if w.Code != 404 {
		t.Errorf("unknown id: %d", w.Code)
	}
}

func mustGet(t *testing.T, lib *library.Library, id string) library.Entry {
	e, ok := lib.Get(id)
	if !ok {
		t.Fatalf("no entry %s", id)
	}
	return e
}

func strconvQuote(p string) string { return strconv.Quote(p) }
