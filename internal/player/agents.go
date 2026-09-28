package player

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Tieumi221E/Jus/capreg"
)

// The guide for coding agents (Jus contract 10): made from the registry,
// so it lists exactly what the command line can do. It is printed; it is
// written into a folder only when asked (-write): a person's media folders
// are not the place for it unasked.

const agentsMark = "<!-- made by jusplay agents; `jusplay agents -write <folder> -force` makes it again -->"

// AgentsGuide is the guide as Markdown.
func AgentsGuide(r *capreg.Registry) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	w("%s\n# Jusplay for agents\n\n", agentsMark)
	w("Jusplay %s plays local videos with Niconico-style comments and subtitles, and keeps a library of media folders. ", r.Version)
	w("Everything its window can do, the command line can do: `jusplay <group> <verb> …`. `jusplay help -json` describes every command (parameters, kinds, which change files, which need the window).\n\n")
	w("## Rules\n\n")
	w("- Add `-json` for machine output (errors too: `{\"error\",\"kind\"}`). Exit codes: 0 ok, 1 failed, 2 usage or `-yes` needed, 3 conflict.\n")
	w("- Say who you are: `-harness <name> -run <session id> [-model <provider/model>]`, or `JUS_HARNESS` / `JUS_RUN` / `JUS_MODEL`. Every change is logged with it; `jusplay changes list -session <id>` shows yours, `jusplay changes revert -session <id> -yes` undoes them (never someone else's).\n")
	w("- A video is its path, its id (`library list`), or a link `jus://play/<folder id>/<path>?t=<seconds>` (`link make`); links survive a moved folder.\n")
	w("- Changes based on what you read: pass `-base <version>` (from `progress get` / `entry info`); exit 3 means it changed, read again.\n")
	w("- Commands marked *window* run in the open Jusplay window (it is started when missing): playback, thumbnails, AI subtitles. The window shows every change at once; `jusplay events watch` streams what happens there.\n")
	w("- A change to a file outside the added folders is not saved (`\"saved\": false`): `library add <folder>` first.\n")
	w("- Commands marked *confirm* want `-yes`; without it they print what they would do.\n")
	w("- Tools for comment snapshots and MKV attachments (import-xml, derive, attach, embed …) are listed by `jusplay help`.\n\n")
	w("## Commands\n\n")
	groups := map[string][]*capreg.Cap{}
	var names []string
	for _, c := range r.All() {
		g := c.Words()[0]
		if _, ok := groups[g]; !ok {
			names = append(names, g)
		}
		groups[g] = append(groups[g], c)
	}
	sort.Strings(names)
	for _, g := range names {
		w("### %s\n\n", g)
		for _, c := range groups[g] {
			var tags []string
			if c.Writes {
				tags = append(tags, "writes")
			}
			if c.Window {
				tags = append(tags, "window")
			}
			if c.Confirm {
				tags = append(tags, "confirm")
			}
			if c.Streams {
				tags = append(tags, "stream")
			}
			t := ""
			if len(tags) > 0 {
				t = " *(" + strings.Join(tags, ", ") + ")*"
			}
			w("- `jusplay %s` — %s%s\n", c.Synopsis(), c.Summary, t)
		}
		w("\n")
	}
	return b.String()
}

func registerAgents(r *capreg.Registry) {
	r.Add(capreg.Cap{ID: "agents", Summary: "the guide for coding agents (Markdown, made from this command list); -write puts it in a folder as AGENTS.md",
		Params: []capreg.Param{
			{Name: "write", Kind: capreg.Path, Doc: "write AGENTS.md into this folder"},
			{Name: "force", Kind: capreg.Bool, Doc: "replace an AGENTS.md this command did not make"}},
		Run: func(ctx context.Context, a capreg.Args) (any, error) {
			guide := AgentsGuide(r)
			if !a.Has("write") {
				return guide, nil
			}
			p := filepath.Join(a.String("write"), "AGENTS.md")
			if old, err := os.ReadFile(p); err == nil && !strings.HasPrefix(string(old), agentsMark) && !a.Bool("force") {
				return nil, errors.New(p + " exists and was not made by jusplay agents: add -force to replace it")
			}
			if err := os.WriteFile(p, []byte(guide), 0o644); err != nil {
				return nil, err
			}
			return map[string]any{"written": p}, nil
		},
		Text: func(w io.Writer, v any) {
			if s, ok := v.(string); ok {
				fmt.Fprint(w, s)
				return
			}
			fmt.Fprintln(w, v.(map[string]any)["written"])
		}})
}
