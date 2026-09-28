package player

// The change log (Jus contract 5): every change made through a capability
// — by the window, the command line or an agent — is kept with who made it
// (the harness and its session, as far as they said) and, where it can be
// put back, what it was before and after. `changes list` shows them;
// `changes revert -session <id>` undoes one session's changes, each only
// if nothing changed it since, so nobody else's change is lost.
//
// A video's changes are kept in its media folder's records
// (.jusplay/changes.jsonl), so they travel with it; the app's own
// (settings, choices, folders) in the data folder (ChangesPath). Changes to
// a file outside every folder are kept in memory only, like its records.
// The window saves settings as they are moved (a slider): those come
// together into one change, written once the moving stops.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Tieumi221E/Jus/capreg"
	"github.com/Tieumi221E/Jusplay/internal/library"
)

// Change is one line of a change log.
type Change struct {
	Time   time.Time      `json:"time"`
	Source capreg.Source  `json:"source"`
	Cap    string         `json:"cap"`
	Params map[string]any `json:"params,omitempty"`
	// A video's choices before and after (see library.Choices).
	Entry        string           `json:"entry,omitempty"`
	Path         string           `json:"path,omitempty"`
	Before       *library.Choices `json:"before,omitempty"`
	After        *library.Choices `json:"after,omitempty"`
	AfterVersion string           `json:"afterVersion,omitempty"`
	// Settings or choices of the app, before and after.
	BeforeDoc json.RawMessage `json:"beforeDoc,omitempty"`
	AfterDoc  json.RawMessage `json:"afterDoc,omitempty"`
	// Revertible: changes revert can put it back.
	Revertible bool `json:"revertible"`
	// Reverts, on a change made by changes revert: the session undone.
	Reverts string `json:"reverts,omitempty"`

	file string // where it is kept ("" memory)
}

// The capabilities whose changes to a video can be put back.
var revertibleEntry = map[string]bool{"entry.watched": true, "progress.set": true, "comments.source": true, "comments.offset": true, "subs.pick": true}

type changeLog struct {
	mu      sync.Mutex
	memory  []Change // changes to files outside every folder
	pending *Change  // the window's settings changes, coming together
	timer   *time.Timer
}

const settingsQuiet = 5 * time.Second

// before is what a change may alter, taken before it runs.
type before struct {
	entry *library.Entry
	doc   json.RawMessage
}

func (s *Server) beforeChange(id string, a capreg.Args) before {
	var b before
	switch {
	case a.Has("entry"):
		if e, err := s.resolve(a.String("entry")); err == nil {
			b.entry = &e
		}
	case id == "settings.set":
		b.doc = s.settings.Get()
	case id == "prefs.set":
		b.doc, _ = json.Marshal(s.prefs.get())
	}
	return b
}

// logChange keeps the change a capability just made.
func (s *Server) logChange(ctx context.Context, id string, a capreg.Args, b before) {
	c := Change{Time: time.Now(), Source: capreg.SourceOf(ctx), Cap: id, Params: map[string]any(a)}
	switch {
	case b.entry != nil:
		e, ok := s.lib.Get(b.entry.ID)
		if !ok {
			return
		}
		pre, post := b.entry.Choices(), e.Choices()
		c.Entry, c.Path, c.Before, c.After, c.AfterVersion = e.ID, e.Path, &pre, &post, e.Version()
		c.Revertible = revertibleEntry[id]
		if d := s.lib.DataDir(e); d != "" {
			c.file = filepath.Join(d, "changes.jsonl")
		}
	case b.doc != nil:
		c.BeforeDoc = b.doc
		if id == "settings.set" {
			c.AfterDoc = s.settings.Get()
		} else {
			c.AfterDoc, _ = json.Marshal(s.prefs.get())
		}
		c.Revertible, c.file = true, s.ChangesPath
	default:
		c.file = s.ChangesPath
	}
	s.changes.mu.Lock()
	defer s.changes.mu.Unlock()
	if id == "settings.set" && c.Source.Via == "window" {
		// Coming together while the window keeps saving.
		if p := s.changes.pending; p != nil {
			p.AfterDoc, p.Time = c.AfterDoc, c.Time
			s.changes.timer.Reset(settingsQuiet)
			return
		}
		s.changes.pending = &c
		s.changes.timer = time.AfterFunc(settingsQuiet, s.flushPending)
		return
	}
	s.flushPendingLocked()
	s.writeChange(c)
}

func (s *Server) flushPending() {
	s.changes.mu.Lock()
	defer s.changes.mu.Unlock()
	s.flushPendingLocked()
}

func (s *Server) flushPendingLocked() {
	if p := s.changes.pending; p != nil {
		s.changes.pending = nil
		s.changes.timer.Stop()
		if !bytes.Equal(p.BeforeDoc, p.AfterDoc) {
			s.writeChange(*p)
		}
	}
}

// writeChange appends c to its log; callers hold changes.mu.
func (s *Server) writeChange(c Change) {
	if c.file == "" {
		s.changes.memory = append(s.changes.memory, c)
		return
	}
	b, err := json.Marshal(c)
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(c.file), 0o755)
	f, err := os.OpenFile(c.file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		s.logf("change log %s: %v", c.file, err)
		s.changes.memory = append(s.changes.memory, c)
		return
	}
	defer f.Close()
	f.Write(append(b, '\n')) // one write: a line is whole or absent
}

// Changes are all changes kept, oldest first.
func (s *Server) Changes() []Change {
	s.changes.mu.Lock()
	s.flushPendingLocked()
	out := append([]Change(nil), s.changes.memory...)
	s.changes.mu.Unlock()
	files := []string{}
	if s.ChangesPath != "" {
		files = append(files, s.ChangesPath)
	}
	for _, f := range s.lib.FolderInfos() {
		files = append(files, filepath.Join(f.Path, library.VaultDir, "changes.jsonl"))
	}
	for _, name := range files {
		f, err := os.Open(name)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 16<<20)
		for sc.Scan() {
			var c Change
			if json.Unmarshal(sc.Bytes(), &c) == nil && c.Cap != "" {
				c.file = name
				out = append(out, c)
			}
		}
		f.Close()
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out
}

// RevertStep is one change changes revert would put back, or could not.
type RevertStep struct {
	Time  time.Time `json:"time"`
	Cap   string    `json:"cap"`
	Path  string    `json:"path,omitempty"`
	What  string    `json:"what"`          // "video", "settings" or "choices"
	State string    `json:"state"`         // "ready", "done" or "conflict"
	Why   string    `json:"why,omitempty"` // for a conflict
}

// Revert puts back the revertible changes of session run, newest first,
// each only if what it changed is still as it left it. With apply false it
// only says what it would do.
func (s *Server) Revert(ctx context.Context, run string, apply bool) ([]RevertStep, error) {
	if run == "" {
		return nil, capreg.Usagef("changes revert: give -session <id>")
	}
	all := s.Changes()
	var mine []Change
	for _, c := range all {
		if c.Source.Run == run && c.Revertible && c.Reverts == "" {
			mine = append(mine, c)
		}
	}
	steps := []RevertStep{}
	for i := len(mine) - 1; i >= 0; i-- {
		c := mine[i]
		st := RevertStep{Time: c.Time, Cap: c.Cap, Path: c.Path, State: "ready"}
		switch {
		case c.Entry != "" && c.Before != nil:
			st.What = "video"
			e, ok := s.lib.Get(c.Entry)
			switch {
			case !ok:
				st.State, st.Why = "conflict", "the video is not in the library now"
			case e.Version() != c.AfterVersion:
				st.State, st.Why = "conflict", "changed since (by someone else, or later in this session)"
			case apply:
				if err := s.lib.SetChoices(c.Entry, c.AfterVersion, *c.Before); err != nil {
					st.State, st.Why = "conflict", err.Error()
					break
				}
				st.State = "done"
				after, _ := s.lib.Get(c.Entry)
				pre, post := *c.After, after.Choices()
				s.changes.mu.Lock()
				s.writeChange(Change{Time: time.Now(), Source: capreg.SourceOf(ctx), Cap: "changes.revert", Reverts: run,
					Entry: c.Entry, Path: c.Path, Before: &pre, After: &post, AfterVersion: after.Version(), file: c.file})
				s.changes.mu.Unlock()
			}
		case c.BeforeDoc != nil:
			st.What = "settings"
			cur := s.settings.Get()
			if c.Cap == "prefs.set" {
				st.What = "choices"
				cur, _ = json.Marshal(s.prefs.get())
			}
			switch {
			case !sameJSON(cur, c.AfterDoc):
				st.State, st.Why = "conflict", "changed since (by someone else, or later in this session)"
			case apply:
				var err error
				if c.Cap == "prefs.set" {
					_, err = s.prefs.update(c.BeforeDoc)
				} else {
					err = s.settings.Put(c.BeforeDoc)
				}
				if err != nil {
					st.State, st.Why = "conflict", err.Error()
					break
				}
				st.State = "done"
				s.changes.mu.Lock()
				s.writeChange(Change{Time: time.Now(), Source: capreg.SourceOf(ctx), Cap: "changes.revert", Reverts: run,
					BeforeDoc: c.AfterDoc, AfterDoc: c.BeforeDoc, file: c.file})
				s.changes.mu.Unlock()
				s.publish("changed", capreg.SourceOf(ctx), map[string]any{"cap": c.Cap, "reverts": run})
			}
		}
		if st.What != "" {
			steps = append(steps, st)
		}
	}
	if apply {
		s.publish("changed", capreg.SourceOf(ctx), map[string]any{"cap": "changes.revert", "run": run})
	}
	return steps, nil
}

func sameJSON(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return bytes.Equal(a, b)
	}
	ja, _ := json.Marshal(x)
	jb, _ := json.Marshal(y)
	return bytes.Equal(ja, jb)
}
