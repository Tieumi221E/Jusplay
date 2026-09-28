package player

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Tieumi221E/Jus/capreg"
	"github.com/Tieumi221E/Jusplay/internal/ai"
	"github.com/Tieumi221E/Jusplay/internal/danmaku"
	"github.com/Tieumi221E/Jusplay/internal/library"
	"github.com/Tieumi221E/Jusplay/internal/safeswap"
)

// Comment translation. Only the comments the player shows are translated
// (after its filters and cap: on real episodes 15–30 % of them), and one
// translation serves every writing of a line: laughter at the end and
// doubling are cut off and put back, kana spellings are merged, common
// reactions come from a table. Lines just ahead of the playhead go first.
// Translations are kept in the video's folder records.
//
//	POST api/ctranslate   {"id", "target", "pos", "shown"?}: start, move the priority, set the lines shown
//	GET  api/ctranslate?id=&target=&since=   translations made after version since
//	POST api/ctranslate/stop

const (
	ctAhead      = 90 // seconds ahead of the playhead translated first
	ctSaveEach   = 25
	ctSchema     = "jusplay-comment-translations/1"
	glossarySize = 30
)

// ctUnit is one thing to translate: a line's core in all its writings.
type ctUnit struct {
	shape string // danmaku.ShapeKey of the core: the unit's identity
	text  string // the core in its most said writing, sent to the model
	count int    // shown comments it serves
}

// ctBody is how a shown comment's text is rebuilt from its unit's translation.
type ctBody struct {
	unit   *ctUnit
	tail   string
	repeat int
}

type ctJob struct {
	id, target, path string

	mu      sync.Mutex
	pos     float64
	done    map[string]string // unit shape → translation of its core
	order   []string          // shapes in the order translated (index+1 is the version)
	busy    map[string]bool
	err     string
	model   string
	running bool
	cancel  context.CancelFunc

	c        *danmaku.Corpus
	seconds  int
	units    []*ctUnit            // to translate, most said first
	bySecond [][]*ctUnit          // units said in each second
	bodies   map[string]ctBody    // shown comment text → how to rebuild it
	byUnit   map[*ctUnit][]string // unit → its comment texts
	needN    int                  // shown comments that need translating

	glossary map[string]string // names and terms, translated once for every line
	gTerms   []string          // their source forms, longest first
}

type ctFile struct {
	Schema   string            `json:"schema"`
	Target   string            `json:"target"`
	Model    string            `json:"model"`
	Made     time.Time         `json:"made"`
	Tr       map[string]string `json:"tr"`                 // by unit shape
	Glossary map[string]string `json:"glossary,omitempty"` // the episode's names and terms
}

// needsTranslation reports whether a phrase in language lang needs
// translating into target. Chinese characters alone read in both Chinese
// and Japanese; lines without letters need nothing.
func needsTranslation(lang, target string) bool {
	switch target {
	case "zh", "zh-Hant":
		return lang == danmaku.LangJa || lang == danmaku.LangEn || lang == danmaku.LangKo
	case "ja":
		return lang == danmaku.LangZh || lang == danmaku.LangEn || lang == danmaku.LangKo
	case "en":
		return lang == danmaku.LangJa || lang == danmaku.LangZh || lang == danmaku.LangKo || lang == danmaku.LangHan
	case "ko":
		return lang == danmaku.LangJa || lang == danmaku.LangZh || lang == danmaku.LangEn || lang == danmaku.LangHan
	}
	return false
}

func (s *Server) ctPath(e library.Entry, target string) string {
	d := s.cacheDir(e, "translations")
	if d == "" {
		return ""
	}
	return filepath.Join(d, e.ID+"."+target+".json")
}

func newCTJob(s *Server, sess *Session, e library.Entry, target string) (*ctJob, error) {
	c, err := danmaku.Load(bytes.NewReader(sess.Comments.playback))
	if err != nil {
		return nil, err
	}
	j := &ctJob{id: sess.ID, target: target, path: s.ctPath(e, target), done: map[string]string{}, busy: map[string]bool{},
		c: c, seconds: int(sess.Media.Duration) + 1, glossary: map[string]string{}}
	if b, err := os.ReadFile(j.path); err == nil {
		var f ctFile
		if json.Unmarshal(b, &f) == nil && f.Schema == ctSchema && f.Target == target {
			for k, v := range f.Tr {
				j.done[k] = v
				j.order = append(j.order, k)
			}
			for k, v := range f.Glossary {
				j.glossary[k] = v
			}
			j.model = f.Model
		}
	}
	for _, t := range c.Terms(80) {
		if len(j.gTerms) < glossarySize && hasKatakana(t.Text) && needsTranslation(danmaku.Lang(t.Text), target) {
			j.gTerms = append(j.gTerms, t.Text)
		}
	}
	sort.Slice(j.gTerms, func(a, b int) bool { return len(j.gTerms[a]) > len(j.gTerms[b]) })
	j.setShown(nil)
	return j, nil
}

// setShown rebuilds the units from the comments shown (nil: all of them),
// keeping every translation made. The caller holds j.mu, or owns j.
func (j *ctJob) setShown(shown []string) {
	var only map[string]bool
	if shown != nil {
		only = make(map[string]bool, len(shown))
		for _, b := range shown {
			only[b] = true
		}
	}
	c := j.c
	units := map[string]*ctUnit{}
	writing := map[*ctUnit]map[string]int{} // core writings and how often each is said
	j.bodies, j.byUnit, j.needN = map[string]ctBody{}, map[*ctUnit][]string{}, 0
	j.bySecond = make([][]*ctUnit, j.seconds)
	type keyInfo struct {
		need       bool
		core, tail string
		repeat     int
		shape      string
	}
	info := map[uint32]*keyInfo{}
	for i, k := range c.Key {
		body := c.Bodies[c.Body[i]]
		if c.Flags[i]&(danmaku.Owner|danmaku.Art) != 0 || only != nil && !only[body] {
			continue
		}
		ki := info[k]
		if ki == nil {
			ki = &keyInfo{need: needsTranslation(danmaku.Lang(c.Keys[k]), j.target)}
			if ki.need {
				ki.core, ki.tail, ki.repeat = danmaku.TranslationCore(c.Keys[k])
				ki.shape = danmaku.ShapeKey(ki.core)
			}
			info[k] = ki
		}
		if !ki.need {
			continue
		}
		u := units[ki.shape]
		if u == nil {
			u = &ctUnit{shape: ki.shape}
			units[ki.shape] = u
			writing[u] = map[string]int{}
		}
		u.count++
		writing[u][ki.core]++
		j.needN++
		if _, seen := j.bodies[body]; !seen {
			j.bodies[body] = ctBody{unit: u, tail: ki.tail, repeat: ki.repeat}
			j.byUnit[u] = append(j.byUnit[u], body)
		}
		if sec := int(c.Vpos[i] / 1000); sec < j.seconds {
			if l := j.bySecond[sec]; len(l) == 0 || l[len(l)-1] != u {
				j.bySecond[sec] = append(l, u)
			}
		}
	}
	j.units = j.units[:0]
	table := danmaku.Reactions[j.target]
	for _, u := range units {
		best := 0
		for w, n := range writing[u] {
			if n > best || n == best && w < u.text {
				u.text, best = w, n
			}
		}
		if tr, ok := table[u.shape]; ok {
			if _, have := j.done[u.shape]; !have {
				j.done[u.shape] = tr
				j.order = append(j.order, u.shape)
			}
		}
		j.units = append(j.units, u)
	}
	sort.Slice(j.units, func(a, b int) bool {
		return j.units[a].count > j.units[b].count || j.units[a].count == j.units[b].count && j.units[a].shape < j.units[b].shape
	})
}

// next picks the unit to translate now: the most said untranslated one
// just ahead of the playhead, else the most said anywhere.
func (j *ctJob) next() (*ctUnit, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	free := func(u *ctUnit) bool {
		_, ok := j.done[u.shape]
		return !ok && !j.busy[u.shape]
	}
	var best *ctUnit
	for sec := max(0, int(j.pos)); sec < min(len(j.bySecond), int(j.pos)+ctAhead); sec++ {
		for _, u := range j.bySecond[sec] {
			if (best == nil || u.count > best.count) && free(u) {
				best = u
			}
		}
	}
	if best == nil {
		for _, u := range j.units {
			if free(u) {
				best = u
				break
			}
		}
	}
	if best == nil {
		return nil, false
	}
	j.busy[best.shape] = true
	return best, true
}

func (j *ctJob) save() {
	j.mu.Lock()
	f := ctFile{Schema: ctSchema, Target: j.target, Model: j.model, Made: time.Now().UTC().Truncate(time.Second),
		Tr: make(map[string]string, len(j.done)), Glossary: make(map[string]string, len(j.glossary))}
	for k, v := range j.done {
		f.Tr[k] = v
	}
	for k, v := range j.glossary {
		f.Glossary[k] = v
	}
	j.mu.Unlock()
	if j.path == "" {
		return
	}
	b, _ := json.Marshal(f)
	os.MkdirAll(filepath.Dir(j.path), 0o755)
	tmp := safeswap.Temp(j.path)
	if os.WriteFile(tmp, b, 0o644) != nil {
		return
	}
	if _, err := os.Stat(j.path); os.IsNotExist(err) {
		os.Rename(tmp, j.path)
		return
	}
	safeswap.Swap(tmp, j.path)
}

func (j *ctJob) run(ctx context.Context, eng *ai.Engine, logf func(string, ...any)) {
	_, mt := eng.Models()
	j.mu.Lock()
	j.model, j.err = mt, ""
	j.mu.Unlock()
	t0, made := time.Now(), 0
	var wg sync.WaitGroup
	// The glossary first, side by side.
	sem := make(chan struct{}, eng.Parallel())
	for _, term := range j.gTerms {
		j.mu.Lock()
		_, have := j.glossary[term]
		j.mu.Unlock()
		if have {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			if out, err := eng.Translate(ctx, term, j.target); err == nil && out != "" {
				j.mu.Lock()
				j.glossary[term] = out
				j.mu.Unlock()
			}
		}()
	}
	wg.Wait()
	// A glossary term that is a whole line needs no second translation.
	j.mu.Lock()
	for _, u := range j.units {
		if tr, ok := j.glossary[u.text]; ok {
			if _, done := j.done[u.shape]; !done {
				j.done[u.shape] = tr
				j.order = append(j.order, u.shape)
			}
		}
	}
	j.mu.Unlock()
	var madeMu sync.Mutex
	for w := 0; w < eng.Parallel(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				u, ok := j.next()
				if !ok {
					return
				}
				out, err := eng.Translate(ctx, u.text, j.target, j.termsIn(u.text)...)
				j.mu.Lock()
				delete(j.busy, u.shape)
				if err != nil {
					if ctx.Err() == nil {
						j.err = err.Error()
					}
					j.mu.Unlock()
					return
				}
				j.done[u.shape] = out
				j.order = append(j.order, u.shape)
				j.mu.Unlock()
				madeMu.Lock()
				made++
				if made%ctSaveEach == 0 {
					j.save()
				}
				madeMu.Unlock()
			}
		}()
	}
	wg.Wait()
	j.save()
	j.mu.Lock()
	j.running = false
	j.mu.Unlock()
	logf("ctranslate %s → %s: %d lines by the model in %s", j.id, j.target, made, time.Since(t0).Round(time.Second))
}

func hasKatakana(s string) bool {
	for _, r := range s {
		if r >= 'ァ' && r <= 'ヺ' {
			return true
		}
	}
	return false
}

// termsIn returns the glossary entries in a line (at most five, longest
// first; a term inside a longer one found is skipped).
func (j *ctJob) termsIn(line string) [][2]string {
	j.mu.Lock()
	defer j.mu.Unlock()
	var out [][2]string
	var found []string
	for _, t := range j.gTerms {
		tr, ok := j.glossary[t]
		if !ok || !strings.Contains(line, t) {
			continue
		}
		inside := false
		for _, f := range found {
			if strings.Contains(f, t) {
				inside = true
			}
		}
		if inside {
			continue
		}
		found = append(found, t)
		out = append(out, [2]string{t, tr})
		if len(out) == 5 {
			break
		}
	}
	return out
}

func (s *Server) ctranslateAPI(w http.ResponseWriter, r *http.Request, p string) {
	s.ai.mu.Lock()
	eng := s.ai.e
	s.ai.mu.Unlock()
	switch {
	case p == "api/ctranslate" && r.Method == http.MethodPost:
		var req struct {
			ID, Target string
			Pos        float64
			Shown      []string
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if _, ok := ai.Targets[req.Target]; !ok || eng == nil {
			http.Error(w, "unknown target or no AI support", http.StatusBadRequest)
			return
		}
		sess, err := s.Session(req.ID)
		e, ok := s.lib.Get(req.ID)
		if err != nil || !ok || sess.Comments.playback == nil {
			http.Error(w, "no comments", http.StatusNotFound)
			return
		}
		if _, err := s.startCT(eng, sess, e, req.Target, req.Pos, req.Shown); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case p == "api/ctranslate" && r.Method == http.MethodGet:
		q := r.URL.Query()
		s.ctMu.Lock()
		j := s.ct
		s.ctMu.Unlock()
		if j == nil || j.id != q.Get("id") || j.target != q.Get("target") {
			http.Error(w, "not started", http.StatusNotFound)
			return
		}
		since, _ := strconv.Atoi(q.Get("since"))
		j.mu.Lock()
		byShape := make(map[string]*ctUnit, len(j.units))
		translated := 0
		for _, u := range j.units {
			byShape[u.shape] = u
			if _, ok := j.done[u.shape]; ok {
				translated += u.count
			}
		}
		var items [][2]string
		for _, shape := range j.order[min(max(0, since), len(j.order)):] {
			u := byShape[shape]
			if u == nil {
				continue
			}
			for _, b := range j.byUnit[u] {
				cb := j.bodies[b]
				items = append(items, [2]string{b, danmaku.Render(j.done[shape], cb.tail, cb.repeat)})
			}
		}
		out := map[string]any{"version": len(j.order), "items": items, "running": j.running, "error": j.err, "model": j.model,
			"lines": len(j.units), "comments": j.needN, "translatedComments": translated}
		if since == 0 {
			need := make([]string, 0, len(j.bodies))
			for b := range j.bodies {
				need = append(need, b)
			}
			out["need"] = need
		}
		j.mu.Unlock()
		writeJSON(w, out)
	case p == "api/ctranslate/stop" && r.Method == http.MethodPost:
		s.stopCT()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// startCT makes sess's translation into target the one running (another
// one stops), moves its priority to pos, sets the comments shown (nil
// keeps them) and starts it if anything is left to translate.
func (s *Server) startCT(eng *ai.Engine, sess *Session, e library.Entry, target string, pos float64, shown []string) (*ctJob, error) {
	s.ctMu.Lock()
	defer s.ctMu.Unlock()
	j := s.ct
	if j == nil || j.id != sess.ID || j.target != target {
		if j != nil && j.cancel != nil {
			j.cancel()
		}
		var err error
		if j, err = newCTJob(s, sess, e, target); err != nil {
			return nil, err
		}
		s.ct = j
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.pos = pos
	if shown != nil {
		j.setShown(shown)
	}
	pending := false
	for _, u := range j.units {
		if _, ok := j.done[u.shape]; !ok {
			pending = true
			break
		}
	}
	if !j.running && (pending || len(j.glossary) < len(j.gTerms)) {
		ctx, cancel := context.WithCancel(context.Background())
		j.cancel = cancel
		j.running = true
		go j.run(ctx, eng, s.logf)
	}
	return j, nil
}

// status is how far the translation is.
func (j *ctJob) status() map[string]any {
	j.mu.Lock()
	defer j.mu.Unlock()
	translated := 0
	for _, u := range j.units {
		if _, ok := j.done[u.shape]; ok {
			translated += u.count
		}
	}
	return map[string]any{"target": j.target, "running": j.running, "error": j.err, "model": j.model,
		"lines": len(j.units), "comments": j.needN, "translatedComments": translated, "file": j.path}
}

// TranslateComments translates the comments sess shows under the saved
// settings into target, as the player does while playing, and waits until
// it is done (or ctx ends: the work so far is kept in the folder records).
func (s *Server) TranslateComments(ctx context.Context, sess *Session, e library.Entry, target string) (map[string]any, error) {
	eng := s.aiEngine()
	if eng == nil {
		return nil, capreg.NotFoundf("no AI support in this build")
	}
	if _, ok := ai.Targets[target]; !ok {
		return nil, capreg.Usagef("unknown target %q", target)
	}
	res, err := s.FilterComments(sess, s.settings.Get())
	if err != nil {
		return nil, err
	}
	ts, _ := sess.threadsOf()
	shown := []string{}
	for i, t := range ts {
		for _, k := range res.Keep[i] {
			shown = append(shown, t.Comments[k].Body)
		}
	}
	j, err := s.startCT(eng, sess, e, target, 0, shown)
	if err != nil {
		return nil, err
	}
	for {
		st := j.status()
		if running, _ := st["running"].(bool); !running {
			if msg, _ := st["error"].(string); msg != "" {
				return st, errors.New(msg)
			}
			return st, nil
		}
		select {
		case <-ctx.Done():
			return st, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func (s *Server) stopCT() {
	s.ctMu.Lock()
	defer s.ctMu.Unlock()
	if s.ct != nil && s.ct.cancel != nil {
		s.ct.cancel()
	}
}
