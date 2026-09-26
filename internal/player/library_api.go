package player

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"path/filepath"
	"time"

	"github.com/Tieumi221E/Jusplay/internal/comments"
	"github.com/Tieumi221E/Jusplay/internal/embed"
	"github.com/Tieumi221E/Jusplay/internal/library"
	"github.com/Tieumi221E/Jusplay/internal/media"
)

// LibraryEntry is an entry as the library page sees it.
type LibraryEntry struct {
	library.Entry
	// SameName is true when a same-name .json sits next to the video.
	SameName bool `json:"sameName"`
	// AltOf is set on another version of an episode: the file that stands
	// for it (library.Versions). Versions lists them on that file.
	AltOf    string   `json:"altOf,omitempty"`
	Versions []string `json:"versions,omitempty"`
	// Thumb is true when the thumbnail exists; otherwise the page makes it.
	Thumb bool `json:"thumb"`
}

type libraryState struct {
	Folders  []library.FolderInfo `json:"folders"`
	Entries  []LibraryEntry       `json:"entries"`
	Scanning bool                 `json:"scanning"`
	Progress [2]int64             `json:"progress"`
	LastScan *library.ScanStats   `json:"lastScan"`
	Now      time.Time            `json:"now"`
}

// ScanAsync starts a scan unless one is running.
func (s *Server) ScanAsync() {
	if !s.scanning.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer s.scanning.Store(false)
		st, err := s.lib.Scan()
		if err != nil {
			s.logf("scan: %v", err)
		}
		s.logf("scan: %d folders, %d files, %d added, %d changed, %d missing in %s",
			st.Folders, st.Files, st.Added, st.Changed, st.Missing, st.Took.Round(time.Millisecond))
		for _, r := range st.Recovered {
			s.logf("scan: interrupted replacement put right: %s", r)
		}
		s.mu.Lock()
		s.lastScan = &st
		s.mu.Unlock()
		if n := s.thumbs.prune(s.lib.Entries()); n > 0 {
			s.logf("thumbnails: removed %d stale", n)
		}
		if n := s.pruneIndex(); n > 0 {
			s.logf("media index: removed %d stale", n)
		}
	}()
}

func (s *Server) libraryAPI(w http.ResponseWriter, r *http.Request, p string) {
	decode := func(v any) bool {
		if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(v); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return false
		}
		return true
	}
	switch {
	case p == "api/library" && r.Method == http.MethodGet:
		st := libraryState{Folders: s.lib.FolderInfos(), Scanning: s.scanning.Load(), Now: time.Now()}
		done, total := s.lib.Progress()
		st.Progress = [2]int64{done, total}
		s.mu.Lock()
		st.LastScan = s.lastScan
		s.mu.Unlock()
		all := s.lib.Entries()
		primaryOf, others := library.Versions(all)
		for _, e := range all {
			st.Entries = append(st.Entries, LibraryEntry{Entry: e, SameName: !e.Missing && comments.SameName(e.Path) != "",
				AltOf: primaryOf[e.ID], Versions: others[e.ID], Thumb: !e.Missing && s.thumbs.has(e)})
		}
		if st.Entries == nil {
			st.Entries = []LibraryEntry{}
		}
		writeJSON(w, st)
	case p == "api/library/folders" && r.Method == http.MethodPost:
		var req struct {
			Path   string
			Remove bool
		}
		if !decode(&req) {
			return
		}
		var err error
		if req.Remove {
			err = s.lib.RemoveFolder(req.Path)
		} else {
			err = s.lib.AddFolder(req.Path)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		if !req.Remove {
			s.ScanAsync()
		}
		w.WriteHeader(http.StatusNoContent)
	case p == "api/library/scan" && r.Method == http.MethodPost:
		s.ScanAsync()
		w.WriteHeader(http.StatusAccepted)
	case p == "api/library/watched" && r.Method == http.MethodPost:
		var req struct {
			ID      string
			Watched bool
		}
		if !decode(&req) {
			return
		}
		if err := s.lib.Update(req.ID, func(e *library.Entry) { e.Watched = req.Watched; e.Position = 0 }); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case p == "api/library/embed" && r.Method == http.MethodPost:
		// Embed the same-name .json into the MKV (in place, verified).
		var req struct{ ID string }
		if !decode(&req) {
			return
		}
		e, ok := s.lib.Get(req.ID)
		if !ok {
			http.Error(w, "no such entry", http.StatusNotFound)
			return
		}
		s.forget(req.ID) // no stream may hold the file while it is replaced
		// Comments being replaced are kept in the folder's records.
		root := e.Folder
		if root == "" {
			root = filepath.Dir(e.Path)
		}
		res, err := embed.Embed(e.Path, "", embed.Options{BackupDir: filepath.Join(root, library.VaultDir, "backup")})
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		if st, err := os.Stat(e.Path); err == nil {
			n := res.Comments
			s.lib.Update(req.ID, func(x *library.Entry) {
				x.Size, x.ModTime, x.CommentCount = st.Size(), st.ModTime(), &n
				if x.Probe != nil {
					x.Probe.Attached = true
				}
			})
		}
		s.logf("embed %s: %d comments, %d packets verified in %s; backup %q; recovered %q", e.Path, res.Comments, res.Verification.Packets, res.Took, res.Backup, res.Recovered)
		writeJSON(w, map[string]any{"comments": res.Comments, "replaced": res.Replaced, "tookMs": res.Took.Milliseconds()})
	case p == "api/thumb" && r.Method == http.MethodGet:
		// Made by the page (thumbnailer.ts); a missing one is its to make.
		e, ok := s.lib.Get(r.URL.Query().Get("id"))
		if !ok || e.Missing || !s.thumbs.has(e) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/webp")
		w.Header().Set("Cache-Control", "max-age=86400")
		http.ServeFile(w, r, s.thumbs.path(e))
	case p == "api/thumbsrc" && r.Method == http.MethodGet:
		// What the page makes a thumbnail from: the keyframe a third of the
		// way in, as a one-sample stream (the media index only; no session,
		// no comments).
		e, ok := s.lib.Get(r.URL.Query().Get("id"))
		if !ok || e.Missing {
			http.NotFound(w, r)
			return
		}
		m, err := media.OpenCached(e.Path, s.cacheDir(e, "index"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		st, err := m.OpenStreamN(r.Context(), "video", m.KeyframeAtOrBefore(m.Duration/3), 1)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		h := w.Header()
		h.Set("Content-Type", "video/mp4")
		h.Set("X-Jusplay-Mime", m.Video.MIME)
		h.Set("X-Jusplay-Offset", strconv.FormatFloat(st.Offset, 'f', 6, 64))
		h.Set("X-Jusplay-Start", strconv.FormatFloat(st.Start, 'f', 6, 64))
		st.WriteTo(w)
	case p == "api/thumb" && r.Method == http.MethodPost:
		e, ok := s.lib.Get(r.URL.Query().Get("id"))
		if !ok || e.Missing {
			http.NotFound(w, r)
			return
		}
		b, err := io.ReadAll(io.LimitReader(r.Body, maxThumb+1))
		if err == nil {
			err = s.thumbs.put(e, b)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}
