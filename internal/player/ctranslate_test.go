package player

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Tieumi221E/Jusplay/internal/ai"
	"github.com/Tieumi221E/Jusplay/internal/danmaku"
	"github.com/Tieumi221E/Jusplay/internal/library"
)

func TestNeedsTranslation(t *testing.T) {
	for _, c := range []struct {
		lang, target string
		want         bool
	}{{"ja", "zh", true}, {"han", "zh", false}, {"none", "zh", false}, {"zh", "zh", false}, {"zh", "ja", true}, {"han", "ja", false}, {"en", "ja", true}} {
		if got := needsTranslation(c.lang, c.target); got != c.want {
			t.Errorf("needsTranslation(%s, %s) = %v", c.lang, c.target, got)
		}
	}
}

func TestCommentTranslation(t *testing.T) {
	// A service that translates by prefixing, and remembers the order asked.
	var mu sync.Mutex
	var asked []string
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Messages []struct{ Content string } }
		json.NewDecoder(r.Body).Decode(&body)
		line := body.Messages[len(body.Messages)-1].Content
		mu.Lock()
		asked = append(asked, line)
		mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "译:" + line}}}})
	}))
	defer svc.Close()

	dir := t.TempDir()
	video := filepath.Join(dir, "S - 01.mkv")
	b, _ := os.ReadFile("../../testdata/media/tiny.mkv")
	os.WriteFile(video, b, 0o644)
	// Comments: a common line early, a rarer one just ahead of 0 s, a line
	// of Chinese characters only, laughter, and comment art.
	type cm struct {
		ID          string   `json:"id"`
		No          int      `json:"no"`
		VposMs      int      `json:"vposMs"`
		Body        string   `json:"body"`
		Commands    []string `json:"commands"`
		UserID      string   `json:"userId"`
		IsPremium   bool     `json:"isPremium"`
		Score       int      `json:"score"`
		PostedAt    string   `json:"postedAt"`
		NicoruCount int      `json:"nicoruCount"`
		NicoruID    *string  `json:"nicoruId"`
		Source      string   `json:"source"`
		IsMyPost    bool     `json:"isMyPost"`
	}
	var cs []cm
	for i := 0; i < 5; i++ {
		cs = append(cs, cm{VposMs: 500, Body: "よく言うセリフ", UserID: "a"})
	}
	cs = append(cs, cm{VposMs: 800, Body: "すこし珍しい", UserID: "b"}, cm{VposMs: 600, Body: "最高", UserID: "c"},
		cm{VposMs: 700, Body: "wwww", UserID: "d"}, cm{VposMs: 900, Body: "アート", Commands: []string{"ca"}, UserID: "e"})
	for i := range cs {
		cs[i].ID, cs[i].No, cs[i].PostedAt, cs[i].Source = fmt.Sprint("t", i+1), i+1, "2026-01-01T00:00:00+09:00", "trunk"
		if cs[i].Commands == nil {
			cs[i].Commands = []string{}
		}
	}
	raw, _ := json.Marshal([]any{map[string]any{"id": 0, "fork": "main", "commentCount": len(cs), "comments": cs}})
	os.WriteFile(filepath.Join(dir, "S - 01.json"), raw, 0o644)

	lib, _ := library.Open(filepath.Join(dir, "folders.json"))
	e, err := lib.Ensure(video)
	if err != nil {
		t.Fatal(err)
	}
	s := New(lib, fstest.MapFS{}, OpenSettings(""), "", t.TempDir())
	eng := ai.New(t.TempDir(), "")
	if err := eng.SetConfig(ai.PublicConfig{ASR: ai.PublicBackend{Kind: "recommended"}, MT: ai.PublicBackend{Kind: "api", URL: svc.URL, Model: "m"}}); err != nil {
		t.Fatal(err)
	}
	s.SetAI(eng)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest(method, "/"+s.token+"/"+path, strings.NewReader(body)))
		return w
	}
	// The order (requests to a service run side by side, so it is checked
	// on a job directly): the most said line ahead of the playhead first.
	sess, _ := s.Session(e.ID)
	probe, err := newCTJob(s, sess, e, "zh")
	if err != nil {
		t.Fatal(err)
	}
	u1, _ := probe.next()
	u2, _ := probe.next()
	if _, more := probe.next(); u1 == nil || u2 == nil || u1.text != "よく言うセリフ" || u2.text != "すこし珍しい" || more {
		t.Errorf("order %+v %+v (more: %v)", u1, u2, more)
	}
	if w := call("POST", "api/ctranslate", `{"id":"`+e.ID+`","target":"zh","pos":0}`); w.Code != 204 {
		t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	var got struct {
		Version  int
		Items    [][2]string
		Need     []string
		Running  bool
		Comments int
		Error    string
	}
	for i := 0; i < 100; i++ {
		w := call("GET", "api/ctranslate?id="+e.ID+"&target=zh&since=0", "")
		json.Unmarshal(w.Body.Bytes(), &got)
		if !got.Running && got.Version >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got.Error != "" || got.Version != 2 || got.Comments != 6 || len(got.Need) != 2 {
		t.Fatalf("state %+v", got)
	}
	tr := map[string]string{}
	for _, it := range got.Items {
		tr[it[0]] = it[1]
	}
	if tr["よく言うセリフ"] != "译:よく言うセリフ" || tr["最高"] != "" || tr["wwww"] != "" || tr["アート"] != "" {
		t.Errorf("translations %v", tr)
	}
	if len(asked) != 2 {
		t.Errorf("asked %v", asked)
	}
	// Kept in the folder records: a new job reads them and asks nothing.
	s.stopCT()
	s.ctMu.Lock()
	s.ct = nil
	s.ctMu.Unlock()
	asked = nil
	call("POST", "api/ctranslate", `{"id":"`+e.ID+`","target":"zh","pos":0}`)
	time.Sleep(100 * time.Millisecond)
	w := call("GET", "api/ctranslate?id="+e.ID+"&target=zh&since=0", "")
	json.Unmarshal(w.Body.Bytes(), &got)
	if got.Version != 2 || len(asked) != 0 {
		t.Errorf("after reopening: version %d, asked %v", got.Version, asked)
	}
}

func TestCommentTranslationUnits(t *testing.T) {
	// Writings of one line share a translation; laughter and doubling are
	// put back; common reactions come from the table; only shown lines count.
	j := &ctJob{target: "zh", seconds: 10, done: map[string]string{}, busy: map[string]bool{}, glossary: map[string]string{}}
	j.c = danmaku.NewCorpus()
	for _, body := range []string{"すこし珍しいww", "すこし珍しい", "スコシ珍シイ", "かわいい", "カワイイwww", "隠れた一言", "よく言うセリフよく言うセリフ"} {
		j.c.Add("main", &danmaku.Comment{VposMs: 1000, Body: body, UserID: "u"})
	}
	j.setShown([]string{"すこし珍しいww", "すこし珍しい", "スコシ珍シイ", "かわいい", "カワイイwww", "よく言うセリフよく言うセリフ"})
	if len(j.units) != 3 {
		t.Fatalf("units %+v", j.units)
	}
	if _, hidden := j.bodies["隠れた一言"]; hidden {
		t.Error("a line not shown is to be translated")
	}
	if j.done[danmaku.ShapeKey("かわいい")] != "可爱" {
		t.Errorf("table: %v", j.done)
	}
	u, _ := j.next()
	if u == nil || u.text != "すこし珍しい" || u.count != 3 {
		t.Fatalf("next %+v", u)
	}
	j.done[u.shape] = "有点少见"
	b := j.bodies["すこし珍しいww"]
	if got := danmaku.Render(j.done[b.unit.shape], b.tail, b.repeat); got != "有点少见ww" {
		t.Errorf("render %q", got)
	}
	d := j.bodies["よく言うセリフよく言うセリフ"]
	if d.repeat != 2 || d.unit.text != "よく言うセリフ" {
		t.Errorf("doubling %+v", d)
	}
}

// comments translate (the command line): the comments shown under the
// saved settings — here with an NG word hiding one line — are translated,
// and the call returns when they are done.
func TestTranslateCommentsWithoutThePage(t *testing.T) {
	var mu sync.Mutex
	var asked []string
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Messages []struct{ Content string } }
		json.NewDecoder(r.Body).Decode(&body)
		line := body.Messages[len(body.Messages)-1].Content
		mu.Lock()
		asked = append(asked, line)
		mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "译:" + line}}}})
	}))
	defer svc.Close()
	dir := t.TempDir()
	video := filepath.Join(dir, "S - 01.mkv")
	b, _ := os.ReadFile("../../testdata/media/tiny.mkv")
	os.WriteFile(video, b, 0o644)
	mk := func(i int, body string) map[string]any {
		return map[string]any{"id": fmt.Sprint("t", i), "no": i, "vposMs": 100 * i, "body": body, "commands": []string{}, "userId": fmt.Sprint("u", i),
			"isPremium": false, "score": 0, "postedAt": "2026-01-01T00:00:00+09:00", "nicoruCount": 0, "nicoruId": nil, "source": "trunk", "isMyPost": false}
	}
	raw, _ := json.Marshal([]any{map[string]any{"id": 0, "fork": "main", "commentCount": 2, "comments": []any{mk(1, "よく言うセリフ"), mk(2, "すこし珍しい")}}})
	os.WriteFile(filepath.Join(dir, "S - 01.json"), raw, 0o644)
	lib, _ := library.Open(filepath.Join(dir, "folders.json"))
	e, err := lib.Ensure(video)
	if err != nil {
		t.Fatal(err)
	}
	settings := OpenSettings("")
	settings.Put([]byte(`{"filters":{"ngWords":["珍しい"]}}`))
	s := New(lib, fstest.MapFS{}, settings, "", t.TempDir())
	eng := ai.New(t.TempDir(), "")
	if err := eng.SetConfig(ai.PublicConfig{ASR: ai.PublicBackend{Kind: "recommended"}, MT: ai.PublicBackend{Kind: "api", URL: svc.URL, Model: "m"}}); err != nil {
		t.Fatal(err)
	}
	s.SetAI(eng)
	sess, err := s.Session(e.ID)
	if err != nil {
		t.Fatal(err)
	}
	st, err := s.TranslateComments(context.Background(), sess, e, "zh")
	if err != nil {
		t.Fatal(err)
	}
	if st["running"] != false || st["comments"] != 1 || st["translatedComments"] != 1 {
		t.Fatalf("status %v", st)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(asked, "|") != "よく言うセリフ" {
		t.Fatalf("asked %q: the hidden line must not be translated", asked)
	}
}
