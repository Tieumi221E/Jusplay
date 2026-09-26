// Package player serves the app to its WebView2 window: the library page,
// the player page, per-track fMP4 streams, verified comment data, progress
// and settings.
//
// It listens on a random loopback port and every path starts with a random
// token, so other local processes and web pages cannot read files through it.
package player

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Tieumi221E/Jusplay/internal/bundle"
	"github.com/Tieumi221E/Jusplay/internal/comments"
	"github.com/Tieumi221E/Jusplay/internal/derive"
	"github.com/Tieumi221E/Jusplay/internal/library"
	"github.com/Tieumi221E/Jusplay/internal/media"
	"github.com/Tieumi221E/Jusplay/internal/mkv"
)

// Comments describes the comment data found for a video.
type Comments struct {
	// Source is "manual", "attachment", "same-name" or "none".
	Source string `json:"source"`
	// Path is the comment file for manual and same-name sources.
	Path          string   `json:"path,omitempty"`
	VideoID       string   `json:"videoId,omitempty"`
	SnapshotID    string   `json:"snapshotId,omitempty"`
	CaptureStatus string   `json:"captureStatus,omitempty"`
	CapturedTo    *string  `json:"capturedTo,omitempty"`
	OffsetMs      int64    `json:"offsetMs"`
	SyncVerified  bool     `json:"syncVerified"`
	Forks         []string `json:"forks,omitempty"`
	Count         int      `json:"count"`
	// Error is set when a source was found but failed verification; the
	// video still plays without comments.
	Error string `json:"error,omitempty"`
	// Problems lists sources tried before this one that failed verification.
	Problems []string `json:"problems,omitempty"`
	playback []byte
}

// Session is one opened video.
type Session struct {
	ID       string       `json:"id"`
	Media    *media.Media `json:"media"`
	File     string       `json:"file"`
	Path     string       `json:"path"`
	Folder   string       `json:"folder"`
	Series   string       `json:"series"`
	Season   *int         `json:"season"`
	Episode  *float64     `json:"episode"`
	Title    string       `json:"title"`
	Position float64      `json:"position"`
	Comments Comments     `json:"comments"`
	// Key identifies the video for per-video settings (earlier versions kept
	// the offset in the settings under it; the page moves it to OffsetMs).
	Key string `json:"key"`
	// OffsetMs is the comment offset chosen for this file (library.Entry).
	OffsetMs *int64 `json:"offsetMs,omitempty"`
	// Prev and Next are the neighbouring episodes of the same series.
	Prev string `json:"prev,omitempty"`
	Next string `json:"next,omitempty"`

	mu      sync.Mutex
	cancels map[string]context.CancelFunc // one live stream per track
}

// LoadComments finds comment data for video, in this order:
//
//  1. manual: a file the user chose (or -comments), when set;
//  2. attachment: jusplay attachments embedded in the MKV;
//  3. same-name: video.json next to the video (コメント増量 export).
//
// A source that exists but fails verification is reported in Error and the
// next one is tried, so a broken attachment does not hide a good file.
func LoadComments(video, manual string) Comments {
	var problems []string
	if manual != "" {
		c := fromFile("manual", manual)
		if c.Error == "" {
			return c
		}
		problems = append(problems, "手动选择的文件："+c.Error)
	}
	if c, ok := fromAttachments(video); ok {
		if c.Error == "" {
			c.Problems = problems
			return c
		}
		problems = append(problems, "MKV 附件："+c.Error)
	}
	if p := comments.SameName(video); p != "" {
		c := fromFile("same-name", p)
		if c.Error == "" {
			c.Problems = problems
			return c
		}
		problems = append(problems, "同名 JSON："+c.Error)
	}
	c := Comments{Source: "none", Problems: problems}
	if len(problems) > 0 {
		c.Error = strings.Join(problems, "；")
	}
	return c
}

func fromAttachments(video string) (Comments, bool) {
	c := Comments{Source: "attachment"}
	if !strings.EqualFold(filepath.Ext(video), ".mkv") {
		return c, false // attachments live in Matroska files only
	}
	files, err := mkv.Extract(video)
	if err != nil {
		c.Error = err.Error()
		return c, true
	}
	if len(files) == 0 {
		return c, false
	}
	m, err := bundle.Check(files)
	if m != nil {
		c.VideoID, c.SnapshotID, c.CaptureStatus, c.CapturedTo = m.VideoID, m.SnapshotID, m.CaptureStatus, m.SnapshotCapturedTo
		c.Forks = m.Forks
		if m.Timing.OffsetMs != nil {
			c.OffsetMs = *m.Timing.OffsetMs
		}
		c.SyncVerified = m.Timing.Verified
	}
	if err != nil {
		c.Error = err.Error()
		return c, true
	}
	c.playback = files[bundle.PlaybackName]
	c.Count = countComments(c.playback)
	return c, true
}

func fromFile(source, path string) Comments {
	c := Comments{Source: source, Path: path}
	s, err := comments.Open(path, "")
	if err != nil {
		c.Error = err.Error()
		return c
	}
	c.VideoID, c.SnapshotID, c.CaptureStatus, c.CapturedTo = s.Capture.VideoID, s.Capture.SnapshotID, s.Report.Status, s.Capture.CapturedTo
	playback, rep, err := derive.Derive(s, derive.Options{})
	if err != nil {
		c.Error = err.Error()
		return c
	}
	c.playback, c.Count = playback, rep.Output
	return c
}

func countComments(playback []byte) int {
	var ts []struct {
		Comments []json.RawMessage `json:"comments"`
	}
	if json.Unmarshal(playback, &ts) != nil {
		return 0
	}
	n := 0
	for _, t := range ts {
		n += len(t.Comments)
	}
	return n
}

type Server struct {
	lib      *library.Library
	ui       fs.FS
	settings *Settings
	prefs    *prefStore
	token    string
	ln       net.Listener
	srv      *http.Server
	thumbs   *thumbs
	// temp holds the caches of files that have no media folder records to
	// keep them in (appdir.Temp); "" keeps none.
	temp string

	mu       sync.Mutex
	sessions map[string]*Session // recently opened, newest last in order
	order    []string
	opening  map[string]*sync.WaitGroup
	prewarm  chan string // entries whose index to build in the background
	scanning atomic.Bool
	lastScan *library.ScanStats

	// OnSelftest, if set, receives the report the page posts in ?selftest mode.
	OnSelftest func([]byte)
	// ShotsDir, if set, receives the PNG frames the page posts in ?shots mode.
	ShotsDir string
	// Log receives stream and page events; nil discards them.
	Log *log.Logger
	// LogText, if set, is the log kept so far (memlog), for the page's
	// "copy diagnostic log".
	LogText func() string

	activeStreams atomic.Int64
	openedStreams atomic.Int64
}

const keepSessions = 3

func (s *Server) logf(format string, args ...any) {
	if s.Log != nil {
		s.Log.Printf(format, args...)
	}
}

// New prepares a server; call Start to listen. prefsPath is the UI
// preferences file ("" to keep them in memory); temp is the app's
// temporary folder (appdir.Temp).
func New(lib *library.Library, ui fs.FS, settings *Settings, prefsPath string, temp string) *Server {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	s := &Server{lib: lib, ui: ui, settings: settings, prefs: openPrefs(prefsPath), token: hex.EncodeToString(b[:]),
		sessions: map[string]*Session{}, opening: map[string]*sync.WaitGroup{}, prewarm: make(chan string, 1), temp: temp}
	s.thumbs = newThumbs(func(e library.Entry) string { return s.cacheDir(e, "thumbs") })
	go s.prewarmLoop()
	return s
}

// cacheDir is where e's cached files of a kind ("thumbs", "index") go: its
// media folder's records folder, so they travel with the folder, or the
// temporary folder, emptied when the app closes.
func (s *Server) cacheDir(e library.Entry, kind string) string {
	d := s.lib.DataDir(e)
	if d == "" {
		if s.temp == "" {
			return ""
		}
		d = s.temp
	}
	return filepath.Join(d, kind)
}

// prewarmDelay keeps the next episode's indexing (a pass over the file's
// headers) away from the current episode's start.
const prewarmDelay = 20 * time.Second

// prewarmLoop indexes the next episode of whatever is playing, so that
// "next episode" opens from the index cache. One at a time; a newer request
// replaces a waiting one.
func (s *Server) prewarmLoop() {
	for id := range s.prewarm {
		time.Sleep(prewarmDelay)
		e, ok := s.lib.Get(id)
		if !ok || e.Missing {
			continue
		}
		if f := media.CacheFile(s.cacheDir(e, "index"), e.Path); f != "" {
			if _, err := os.Stat(f); err == nil {
				continue
			}
		}
		t0 := time.Now()
		if _, err := media.OpenCached(e.Path, s.cacheDir(e, "index")); err != nil {
			s.logf("prewarm %s: %v", id, err)
			continue
		}
		s.logf("prewarm %s: indexed in %s", id, time.Since(t0).Round(time.Millisecond))
	}
}

func (s *Server) queuePrewarm(id string) {
	if id == "" {
		return
	}
	select {
	case <-s.prewarm: // drop the older request
	default:
	}
	select {
	case s.prewarm <- id:
	default:
	}
}

// pruneIndex removes cached indexes of files that are no longer in the library.
func (s *Server) pruneIndex() (removed int) {
	keep := map[string]map[string]bool{} // per folder
	for _, e := range s.lib.Entries() {
		d := s.cacheDir(e, "index")
		if keep[d] == nil {
			keep[d] = map[string]bool{}
		}
		if f := media.CacheFile(d, e.Path); f != "" {
			keep[d][filepath.Base(f)] = true
		}
	}
	for d, k := range keep {
		if d != "" {
			removed += pruneDir(d, k)
		}
	}
	return removed
}

// PreOpen starts opening a session in the background (while the window is
// still being created), so the page's first request finds it ready.
func (s *Server) PreOpen(id string) {
	go s.Session(id)
}

// Start listens on a random loopback port and returns the base URL.
func (s *Server) Start() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	s.ln = ln
	s.srv = &http.Server{Handler: s}
	go s.srv.Serve(ln)
	return fmt.Sprintf("http://%s/%s/", ln.Addr(), s.token), nil
}

func (s *Server) Close() error {
	s.mu.Lock()
	for _, sess := range s.sessions {
		sess.cancelAll()
	}
	s.mu.Unlock()
	if s.srv == nil {
		return nil
	}
	return s.srv.Close()
}

func (sess *Session) cancelAll() {
	sess.mu.Lock()
	for _, c := range sess.cancels {
		c()
	}
	sess.mu.Unlock()
}

// Session opens (or reuses) the session for a library entry.
func (s *Server) Session(id string) (*Session, error) {
	s.mu.Lock()
	if sess := s.sessions[id]; sess != nil {
		s.mu.Unlock()
		return sess, nil
	}
	// One open per entry at a time: PreOpen and the page's request share it.
	if wg := s.opening[id]; wg != nil {
		s.mu.Unlock()
		wg.Wait()
		s.mu.Lock()
		sess := s.sessions[id]
		s.mu.Unlock()
		if sess != nil {
			return sess, nil
		}
		return s.Session(id)
	}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	s.opening[id] = wg
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.opening, id)
		s.mu.Unlock()
		wg.Done()
	}()
	e, ok := s.lib.Get(id)
	if !ok {
		return nil, fmt.Errorf("no library entry %s", id)
	}
	t0 := time.Now()
	// The media index and the comments are independent: load them together.
	var c Comments
	var cDone sync.WaitGroup
	cDone.Add(1)
	go func() {
		defer cDone.Done()
		c = LoadComments(e.Path, e.Comments)
	}()
	m, err := media.OpenCached(e.Path, s.cacheDir(e, "index"))
	cDone.Wait()
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("file:%s:%d", filepath.Base(e.Path), e.Size)
	if c.VideoID != "" {
		key = "nico:" + c.VideoID
	}
	sess := &Session{ID: id, Media: m, File: filepath.Base(e.Path), Path: e.Path, Folder: e.Folder, Series: e.Series, Season: e.Season, Episode: e.Episode,
		Title: e.Title, Position: e.Position, Comments: c, Key: key, OffsetMs: e.OffsetMs, cancels: map[string]context.CancelFunc{}}
	sess.Prev, sess.Next = s.neighbours(e)
	s.queuePrewarm(sess.Next)
	if c.Source != "none" {
		n := c.Count
		s.lib.Update(id, func(e *library.Entry) { e.CommentCount = &n })
	}
	s.logf("session %s: %s, %d keyframes, comments %s (%d) %s, opened in %s", id, e.Path, len(m.Keyframes),
		c.Source, c.Count, c.Error, time.Since(t0).Round(time.Millisecond))
	s.mu.Lock()
	defer s.mu.Unlock()
	if old := s.sessions[id]; old != nil {
		return old, nil
	}
	s.sessions[id] = sess
	s.order = append(s.order, id)
	for len(s.order) > keepSessions {
		drop := s.order[0]
		s.order = s.order[1:]
		s.sessions[drop].cancelAll()
		delete(s.sessions, drop)
	}
	return sess, nil
}

// sameSeries is e's series (same name within the same added folder), in
// library order, without missing files, one file per episode
// (library.Versions): the others are listed on it. primary is the entry
// that stands for e's episode (e itself unless e is another version).
func (s *Server) sameSeries(e library.Entry) (same []library.Entry, versions map[string][]string, primary string) {
	all := s.lib.Entries()
	primaryOf, others := library.Versions(all)
	primary = e.ID
	if p, ok := primaryOf[e.ID]; ok {
		primary = p
	}
	for _, x := range all {
		if x.Series == e.Series && x.Folder == e.Folder && !x.Missing && primaryOf[x.ID] == "" {
			same = append(same, x)
		}
	}
	return same, others, primary
}

// Episode is one row of the player's episode list (api/episodes).
type Episode struct {
	ID       string   `json:"id"`
	Season   *int     `json:"season"`
	Episode  *float64 `json:"episode"`
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle,omitempty"`
	Duration float64  `json:"duration"`
	Position float64  `json:"position"`
	Watched  bool     `json:"watched"`
	Comments *int     `json:"comments,omitempty"`
	// Versions are the other files of this episode (library.Versions).
	Versions []string `json:"versions,omitempty"`
	// Thumb is the thumbnail URL, versioned like the library page's;
	// HasThumb says whether it exists yet (the page makes missing ones).
	Thumb    string `json:"thumb"`
	HasThumb bool   `json:"hasThumb"`
}

func (s *Server) episodes(w http.ResponseWriter, id string) {
	e, ok := s.lib.Get(id)
	if !ok {
		http.NotFound(w, nil)
		return
	}
	out := struct {
		Series   string    `json:"series"`
		Episodes []Episode `json:"episodes"`
	}{Series: e.Series, Episodes: []Episode{}}
	same, versions, _ := s.sameSeries(e)
	for _, x := range same {
		ep := Episode{ID: x.ID, Season: x.Season, Episode: x.Episode, Title: x.Title, Subtitle: x.Subtitle,
			Position: x.Position, Watched: x.Watched, Comments: x.CommentCount, Versions: versions[x.ID],
			Thumb: fmt.Sprintf("api/thumb?id=%s&v=%d-%d", x.ID, x.Size, x.ModTime.UnixMilli()), HasThumb: !x.Missing && s.thumbs.has(x)}
		if x.Probe != nil {
			ep.Duration = x.Probe.Duration
		}
		out.Episodes = append(out.Episodes, ep)
	}
	writeJSON(w, out)
}

func (s *Server) neighbours(e library.Entry) (prev, next string) {
	same, _, primary := s.sameSeries(e)
	for i, x := range same {
		if x.ID == primary {
			if i > 0 {
				prev = same[i-1].ID
			}
			if i+1 < len(same) {
				next = same[i+1].ID
			}
		}
	}
	return prev, next
}

func (s *Server) forget(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess := s.sessions[id]; sess != nil {
		sess.cancelAll()
		delete(s.sessions, id)
		for i, x := range s.order {
			if x == id {
				s.order = append(s.order[:i], s.order[i+1:]...)
				break
			}
		}
	}
}

// Dark reports whether the saved theme is the dark one (for the window's
// first icon and frame, before the page has loaded).
func (s *Server) Dark() bool { return s.prefs.get().Theme == "dark" }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.ln != nil && r.Host != s.ln.Addr().String() {
		http.Error(w, "bad host", http.StatusForbidden)
		return
	}
	prefix := "/" + s.token + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}
	p := strings.TrimPrefix(r.URL.Path, prefix)
	w.Header().Set("Cache-Control", "no-store")
	if strings.HasPrefix(p, "api/library") || p == "api/thumb" || p == "api/thumbsrc" {
		s.libraryAPI(w, r, p)
		return
	}
	q := r.URL.Query()
	switch {
	case p == "api/session" && r.Method == http.MethodGet:
		sess, err := s.Session(q.Get("id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, sess)
	case strings.HasPrefix(p, "api/stream/") && r.Method == http.MethodGet:
		s.stream(w, r, q.Get("id"), strings.TrimPrefix(p, "api/stream/"))
	case p == "api/episodes" && r.Method == http.MethodGet:
		s.episodes(w, r.URL.Query().Get("id"))
	case p == "api/comments" && r.Method == http.MethodGet:
		sess, err := s.Session(q.Get("id"))
		if err != nil || sess.Comments.playback == nil {
			http.Error(w, "no comments", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(sess.Comments.playback)
	case p == "api/comments/source" && r.Method == http.MethodPost:
		// {"id", "path"} loads a comment file for the video; "" returns to
		// the automatic order. The choice is remembered in the library.
		var req struct{ ID, Path string }
		if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		e, ok := s.lib.Get(req.ID)
		if !ok {
			http.Error(w, "no such entry", http.StatusNotFound)
			return
		}
		c := LoadComments(e.Path, req.Path)
		if req.Path != "" && c.Source != "manual" {
			http.Error(w, c.Error, http.StatusUnprocessableEntity)
			return
		}
		if err := s.lib.Update(req.ID, func(e *library.Entry) { e.Comments = req.Path }); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.forget(req.ID) // reopened with the new source on the next request
		writeJSON(w, c)
	case p == "api/progress" && r.Method == http.MethodPost:
		var req struct {
			ID       string
			Position float64
			Duration float64
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		now := time.Now()
		err := s.lib.Update(req.ID, func(e *library.Entry) {
			e.Position, e.LastPlayed = req.Position, &now
			// Within the last 5 % (credits) counts as watched.
			if req.Duration > 0 && req.Position >= req.Duration*0.95 {
				e.Watched, e.Position = true, 0
			}
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case p == "prefs.js" && r.Method == http.MethodGet:
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Write(s.prefs.get().script())
	case p == "api/prefs" && r.Method == http.MethodPut:
		b, err := io.ReadAll(io.LimitReader(r.Body, 4096))
		var pr Prefs
		if err == nil {
			pr, err = s.prefs.update(b)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pr)
	case p == "api/settings" && r.Method == http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		w.Write(s.settings.Get())
	case p == "api/settings" && r.Method == http.MethodPut:
		b, err := io.ReadAll(io.LimitReader(r.Body, maxSettings+1))
		if err == nil {
			err = s.settings.Put(b)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case p == "api/log" && r.Method == http.MethodGet && s.LogText != nil:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, s.LogText())
	case p == "api/offset" && r.Method == http.MethodPost:
		// The comment offset chosen for a file, kept in its folder's records
		// (null: back to the one the comment data records).
		var req struct {
			ID       string
			OffsetMs *int64
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.OffsetMs != nil && (*req.OffsetMs < -600000 || *req.OffsetMs > 600000) {
			http.Error(w, "offset out of range", http.StatusBadRequest)
			return
		}
		if err := s.lib.Update(req.ID, func(e *library.Entry) { e.OffsetMs = req.OffsetMs }); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.mu.Lock()
		if sess := s.sessions[req.ID]; sess != nil {
			sess.OffsetMs = req.OffsetMs
		}
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case p == "api/log" && r.Method == http.MethodPost:
		// Page-side errors, so a crash of the page leaves evidence.
		b, _ := io.ReadAll(io.LimitReader(r.Body, 16<<10))
		s.logf("page: %s", strings.TrimSpace(string(b)))
		w.WriteHeader(http.StatusNoContent)
	case p == "api/stats" && r.Method == http.MethodGet:
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		writeJSON(w, map[string]any{
			"goroutines": runtime.NumGoroutine(), "heapMB": ms.HeapAlloc >> 20, "sysMB": ms.Sys >> 20,
			"activeStreams": s.activeStreams.Load(), "openedStreams": s.openedStreams.Load(),
		})
	case p == "api/selftest/shot" && r.Method == http.MethodPost && s.ShotsDir != "":
		name := filepath.Base(q.Get("name"))
		if !strings.HasSuffix(name, ".png") || strings.ContainsAny(name, `\/:`) {
			http.Error(w, "bad name", http.StatusBadRequest)
			return
		}
		b, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
		if err == nil {
			err = os.WriteFile(filepath.Join(s.ShotsDir, name), b, 0o644)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case p == "api/selftest" && r.Method == http.MethodPost && s.OnSelftest != nil:
		b, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		w.WriteHeader(http.StatusNoContent)
		go s.OnSelftest(b)
	case r.Method == http.MethodGet:
		s.asset(w, r, p)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) asset(w http.ResponseWriter, r *http.Request, p string) {
	if p == "" {
		p = "library.html"
	}
	if !fs.ValidPath(p) {
		http.NotFound(w, r)
		return
	}
	b, err := fs.ReadFile(s.ui, p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ct := map[string]string{".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8",
		".css": "text/css; charset=utf-8", ".svg": "image/svg+xml", ".woff2": "font/woff2", ".png": "image/png"}[filepath.Ext(p)]
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	// Built assets are content-stable per binary; let the page cache them.
	if p != "library.html" && p != "index.html" {
		w.Header().Set("Cache-Control", "max-age=3600")
	}
	w.Write(b)
}

// stream serves one track of a session from the keyframe at or before ?t=.
// A new request for a track cancels the previous one (the page seeked).
func (s *Server) stream(w http.ResponseWriter, r *http.Request, id, kind string) {
	if kind != "video" && kind != "audio" {
		http.NotFound(w, r)
		return
	}
	sess, err := s.Session(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	t, err := strconv.ParseFloat(r.URL.Query().Get("t"), 64)
	if err != nil {
		t = 0
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	sess.mu.Lock()
	if prev := sess.cancels[kind]; prev != nil {
		prev()
	}
	sess.cancels[kind] = cancel
	sess.mu.Unlock()

	k := sess.Media.KeyframeAtOrBefore(t)
	t0 := time.Now()
	// frames=N: only N samples (the thumbnailer asks for one keyframe).
	frames, _ := strconv.Atoi(r.URL.Query().Get("frames"))
	st, err := sess.Media.OpenStreamN(ctx, kind, k, max(0, frames))
	if err != nil {
		if ctx.Err() == nil {
			s.logf("stream %s %s t=%.3f k=%.3f: %v", id, kind, t, k, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	defer st.Close()
	s.openedStreams.Add(1)
	s.activeStreams.Add(1)
	defer s.activeStreams.Add(-1)
	opened := time.Since(t0)
	h := w.Header()
	h.Set("Content-Type", kind+"/mp4")
	h.Set("X-Jusplay-Offset", strconv.FormatFloat(st.Offset, 'f', 6, 64))
	h.Set("X-Jusplay-Start", strconv.FormatFloat(st.Start, 'f', 6, 64))
	h.Set("X-Jusplay-Keyframe", strconv.FormatFloat(k, 'f', 6, 64))
	w.WriteHeader(http.StatusOK)
	n, err := st.WriteTo(flushWriter{w, http.NewResponseController(w)})
	end := "to end"
	if ctx.Err() != nil {
		end = "cancelled"
	} else if err != nil {
		end = "error: " + err.Error()
	}
	s.logf("stream %s %s t=%.3f start=%.3f aligned %d samples, opened in %s, %d MB, %s",
		id, kind, t, st.Start, st.AlignSamples, opened.Round(time.Millisecond), n>>20, end)
}

type flushWriter struct {
	w  io.Writer
	rc *http.ResponseController
}

func (f flushWriter) Write(b []byte) (int, error) {
	n, err := f.w.Write(b)
	if err == nil {
		err = f.rc.Flush()
	}
	return n, err
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil && !errors.Is(err, http.ErrHandlerTimeout) {
		return
	}
}
