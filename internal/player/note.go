package player

// "记一笔": a line about the moment being watched, written into a Jusnote
// notebook through Jusnote's own command line (Jus contract 19: apps call
// each other's capabilities, never each other's code). The line carries a
// jus://play link back to the moment. Where it goes is in the settings
// document (notes.notebook, notes.file), chosen once in the window.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tieumi221E/Jus/apps"
	"github.com/Tieumi221E/Jus/capreg"
	"github.com/Tieumi221E/Jusplay/internal/library"
)

// DefaultNoteFile is the note the lines go to when the settings name none.
const DefaultNoteFile = "Jusplay.md"

// NoteTarget is where notes go: the settings' choice, or the defaults.
func (s *Server) NoteTarget() (notebook, file string) {
	var doc struct {
		Notes struct {
			Notebook string `json:"notebook"`
			File     string `json:"file"`
		} `json:"notes"`
	}
	json.Unmarshal(s.settings.Get(), &doc)
	file = doc.Notes.File
	if file == "" {
		file = DefaultNoteFile
	}
	return doc.Notes.Notebook, file
}

// AddNote writes text, about entry at t (or what the window plays now,
// when ref is ""), into the notebook (or the settings' one).
func (s *Server) AddNote(ctx context.Context, ref string, at *float64, text, notebook, file string) (map[string]any, error) {
	text = strings.Join(strings.Fields(text), " ") // one line
	if text == "" {
		return nil, capreg.Usagef("note add: nothing to write (-text)")
	}
	var e library.Entry
	var t float64
	var err error
	if ref == "" {
		st := s.liveState()
		id, _ := st["id"].(string)
		if id == "" || st["page"] != "player" {
			return nil, capreg.Usagef("note add: no video is playing in the window; name one (-entry)")
		}
		if e, err = s.resolve(id); err != nil {
			return nil, err
		}
		t, _ = st["position"].(float64)
	} else {
		if e, err = s.resolve(ref); err != nil {
			return nil, err
		}
		if lt, ok := linkMoment(ref); ok {
			t = lt
		}
	}
	if at != nil {
		t = *at
	}
	setNotebook, setFile := s.NoteTarget()
	if notebook == "" {
		notebook = setNotebook
	}
	if file == "" {
		file = setFile
	}
	if notebook == "" {
		return nil, capreg.Usagef("note add: which notebook? choose one in the window (记一笔), or give -notebook")
	}
	exe := apps.Find("jusnote")
	if exe == "" {
		return nil, capreg.NotFoundf("Jusnote is not installed (the note needs it)")
	}
	link := s.LinkOf(e, t)
	line := fmt.Sprintf("- %s %s — %s\n", time.Now().Format("2006-01-02"), markdownLink(linkTitle(e, t), link), text)
	tmp, err := os.CreateTemp("", "jusplay-note-*.md")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	tmp.WriteString(line)
	tmp.Close()
	args := []string{"append", filepath.ToSlash(file), "-file", tmp.Name(), "-notebook", notebook}
	// Who writes it is who asked Jusplay; an agent's line waits for review
	// in Jusnote, as its own writes do.
	src := capreg.SourceOf(ctx)
	for _, f := range [][2]string{{"author", src.Author}, {"harness", src.Harness}, {"model", src.Model}, {"run", src.Run}} {
		if f[1] != "" {
			args = append(args, "-"+f[0], f[1])
		}
	}
	if src.Author == "agent" {
		args = append(args, "-no-commit")
	}
	out, err := apps.Call(ctx, exe, args...)
	if err != nil {
		return nil, fmt.Errorf("Jusnote: %w", err)
	}
	var res any
	json.Unmarshal(out, &res)
	return map[string]any{"notebook": notebook, "file": file, "line": strings.TrimSpace(line), "link": link, "at": t, "jusnote": res}, nil
}
