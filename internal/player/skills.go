package player

// A media folder's own tools (Jus contract 15, the Jus module's skills):
// <folder>/.jusplay/skills/<name>, with their results in
// <folder>/.jusplay/out/<name>. Which ones the user said yes to is kept in
// the app's data folder, not in the media folder.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tieumi221E/Jus/capreg"
	"github.com/Tieumi221E/Jus/skills"
	"github.com/Tieumi221E/Jusplay/internal/library"
)

// FolderSkill is a skill and the media folder it came with.
type FolderSkill struct {
	skills.Skill
	Folder string `json:"folder"`
}

func (s *Server) skillTrust() *skills.Trust {
	dir := os.TempDir()
	if s.settings != nil && s.settings.path != "" {
		dir = filepath.Dir(s.settings.path)
	}
	return skills.OpenTrust(dir)
}

// Skills are the skills of every added folder (or of folder only).
func (s *Server) Skills(folder string) ([]FolderSkill, error) {
	out := []FolderSkill{}
	for _, f := range s.lib.Folders() {
		if folder != "" && !strings.EqualFold(filepath.Clean(f), filepath.Clean(folder)) {
			continue
		}
		list, err := skills.List(filepath.Join(f, library.VaultDir), s.skillTrust())
		if err != nil {
			return nil, err
		}
		for _, k := range list {
			out = append(out, FolderSkill{k, f})
		}
	}
	return out, nil
}

// RunSkill runs skill name (of folder, or of the one folder that has it).
func (s *Server) RunSkill(ctx context.Context, name, folder string, yes bool, args []string) (any, error) {
	all, err := s.Skills(folder)
	if err != nil {
		return nil, err
	}
	var found []FolderSkill
	for _, k := range all {
		if k.Name == name {
			found = append(found, k)
		}
	}
	switch {
	case len(found) == 0:
		return nil, capreg.NotFoundf("no skill %s in the added folders (skills list)", name)
	case len(found) > 1:
		return nil, capreg.Usagef("skill %s is in %d folders; say which (-folder)", name, len(found))
	}
	k := found[0]
	if k.Problem != "" || k.Run == nil {
		_, err := skills.Run(ctx, k.Skill, k.Folder, nil, nil)
		return nil, capreg.Usagef("%v", err)
	}
	vault := filepath.Join(k.Folder, library.VaultDir)
	if !k.Trusted {
		if !yes {
			return nil, capreg.NeedConfirm("skill "+k.Name+" runs a program that came with the folder; add -yes to run it (asked again if its files change)",
				map[string]any{"skill": k.Name, "folder": k.Folder, "command": append(k.Command(k.Folder), args...), "dir": k.Dir, "out": k.Out, "hash": k.Hash})
		}
		if err := s.skillTrust().Add(vault, k.Name, k.Hash); err != nil {
			return nil, err
		}
		k.Trusted = true
	}
	exe, _ := os.Executable()
	res, err := skills.Run(ctx, k.Skill, k.Folder, []string{"JUSPLAY_FOLDER=" + k.Folder, "JUSPLAY_EXE=" + exe}, args)
	if err != nil {
		return nil, err
	}
	if res.Exit != 0 || res.TimedOut {
		last := strings.TrimSpace(res.Tail)
		if i := strings.LastIndexByte(last, '\n'); i >= 0 {
			last = last[i+1:]
		}
		if res.TimedOut {
			return nil, fmt.Errorf("skill %s ran out of time (%s)", k.Name, res.Log)
		}
		return nil, fmt.Errorf("skill %s exited %d: %s (all it printed: %s)", k.Name, res.Exit, last, res.Log)
	}
	return res, nil
}
