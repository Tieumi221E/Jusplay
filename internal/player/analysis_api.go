package player

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Tieumi221E/Jusplay/internal/danmaku"
	"github.com/Tieumi221E/Jusplay/internal/library"
	"github.com/Tieumi221E/Jusplay/internal/safeswap"
)

// The comment analysis of a video (package danmaku), for the player's
// hotspot marks and its report:
//
//	GET api/analysis?id=   the report as JSON (404 when there are no comments)
//
// It is made from the comment data as loaded, all forks, before the
// player's filters (the report describes the data, not a view of it), and
// kept in the video's folder records under the data's hash, so it is made
// again only when the comments or the analysis change.

// analysisVersion changes whenever the analysis would give another report.
const analysisVersion = "3"

type analysis struct {
	once sync.Once
	body []byte
	err  error
}

type analysisFile struct {
	Key    string          `json:"key"` // analysisVersion + SHA-256 of the comment data
	Report json.RawMessage `json:"report"`
}

func (s *Server) analysisPath(e library.Entry) string {
	d := s.cacheDir(e, "analysis")
	if d == "" {
		return ""
	}
	return filepath.Join(d, e.ID+".json")
}

func (s *Server) analysisAPI(w http.ResponseWriter, r *http.Request) {
	sess, err := s.Session(r.URL.Query().Get("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if sess.Comments.playback == nil {
		http.Error(w, "no comments", http.StatusNotFound)
		return
	}
	a := sess.analysisOnce()
	a.once.Do(func() {
		sum := sha256.Sum256(sess.Comments.playback)
		key := analysisVersion + ":" + hex.EncodeToString(sum[:])
		e, _ := s.lib.Get(sess.ID)
		path := s.analysisPath(e)
		if b, err := os.ReadFile(path); err == nil {
			var f analysisFile
			if json.Unmarshal(b, &f) == nil && f.Key == key && len(f.Report) > 0 {
				a.body = f.Report
				return
			}
		}
		t0 := time.Now()
		c, err := danmaku.Load(bytes.NewReader(sess.Comments.playback))
		if err != nil {
			a.err = err
			return
		}
		rep := danmaku.Analyze(c, sess.Media.Duration)
		if a.body, a.err = json.Marshal(rep); a.err != nil {
			return
		}
		s.logf("analysis %s: %d comments in %s (%v ms)", sess.ID, c.Len(), time.Since(t0).Round(time.Millisecond), rep.Timing)
		if path != "" {
			b, _ := json.Marshal(analysisFile{Key: key, Report: a.body})
			os.MkdirAll(filepath.Dir(path), 0o755)
			tmp := safeswap.Temp(path)
			if os.WriteFile(tmp, b, 0o644) == nil {
				if _, err := os.Stat(path); os.IsNotExist(err) {
					os.Rename(tmp, path)
				} else {
					safeswap.Swap(tmp, path)
				}
			}
		}
	})
	if a.err != nil {
		http.Error(w, a.err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(a.body)
}

func (sess *Session) analysisOnce() *analysis {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if sess.analysis == nil {
		sess.analysis = &analysis{}
	}
	return sess.analysis
}
