package player

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Tieumi221E/Jus/capreg"
	"github.com/Tieumi221E/Jusplay/internal/ai"
	"github.com/Tieumi221E/Jusplay/internal/library"
	"github.com/Tieumi221E/Jusplay/internal/subs"
)

// Register adds what the window can do to r: the same operations its
// pages use, so the command line (and any agent) can do all of it. get
// gives the server lazily, so `help` does not open the library.
func Register(r *capreg.Registry, get func() (*Server, error)) {
	with := func(run func(ctx context.Context, s *Server, a capreg.Args) (any, error)) func(context.Context, capreg.Args) (any, error) {
		return func(ctx context.Context, a capreg.Args) (any, error) {
			s, err := get()
			if err != nil {
				return nil, err
			}
			return run(ctx, s, a)
		}
	}
	// Every change made through a capability is an event (events.watch),
	// with who made it.
	add := func(c capreg.Cap) {
		if run := c.Run; c.Writes && run != nil {
			id := c.ID
			c.Run = func(ctx context.Context, a capreg.Args) (any, error) {
				s, e := get()
				if e != nil {
					return run(ctx, a)
				}
				pre := s.beforeChange(id, a)
				v, err := run(ctx, a)
				if err != nil {
					return v, err
				}
				s.logChange(ctx, id, a, pre)
				s.publish("changed", capreg.SourceOf(ctx), map[string]any{"cap": id, "params": map[string]any(a)})
				// A change to one video says whether it was kept: a file outside
				// the added folders has its records in memory only, so the
				// change ends with this call when the window is not open.
				if a.Has("entry") {
					if x, err := s.resolve(a.String("entry")); err == nil {
						m, _ := capreg.Generic(v).(map[string]any)
						if m == nil {
							m = map[string]any{"ok": true}
						}
						m["saved"] = s.lib.DataDir(x) != ""
						if !m["saved"].(bool) {
							m["note"] = "not saved: the video is not under an added folder (library add <folder>), so its records are kept in memory only, until the program holding them (the window, or this command) ends"
						}
						return m, nil
					}
				}
				return v, nil
			}
		}
		r.Add(c)
	}
	entry := capreg.Param{Name: "entry", Kind: capreg.Ref, Required: true, Positional: true,
		Doc: "a video: its path, its id from library list, or a jus://play link"}
	base := capreg.Param{Name: "base", Kind: capreg.String,
		Doc: "the entry's version this change is based on (from progress get / entry info); fails with exit 3 if it changed"}

	// ---- library ----

	add(capreg.Cap{ID: "library.list", Summary: "the videos in the library (one row per file) and its folders",
		Params: []capreg.Param{
			{Name: "series", Kind: capreg.String, Doc: "only this series (exact name)"},
			{Name: "folder", Kind: capreg.Path, Doc: "only files under this added folder"},
			{Name: "unwatched", Kind: capreg.Bool, Doc: "only files not watched yet"},
			{Name: "missing", Kind: capreg.Bool, Doc: "also files gone since the last scan"},
		},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			s.lib.Refresh()
			st := s.LibraryState()
			out := []listRow{}
			for _, e := range st.Entries {
				if e.Missing && !a.Bool("missing") ||
					a.Has("series") && e.Series != a.String("series") ||
					a.Has("folder") && !strings.EqualFold(e.Folder, filepath.Clean(a.String("folder"))) ||
					a.Bool("unwatched") && e.Watched {
					continue
				}
				out = append(out, rowOf(e))
			}
			return map[string]any{"folders": st.Folders, "entries": out, "scanning": st.Scanning}, nil
		}),
		Text: printList})

	add(capreg.Cap{ID: "library.folders", Summary: "the added media folders and whether their records can be saved",
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			return s.lib.FolderInfos(), nil
		})})

	add(capreg.Cap{ID: "library.add", Summary: "add a media folder (its records come with it), then scan", Writes: true,
		Params: []capreg.Param{{Name: "folder", Kind: capreg.Path, Required: true, Positional: true, Doc: "the folder"}, {Name: "wait", Kind: capreg.Bool, Default: true, Doc: "wait for the scan (false: scan in the background; library list shows scanning)"}},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			if err := s.lib.AddFolder(a.String("folder")); err != nil {
				return nil, err
			}
			if !a.Bool("wait") {
				s.ScanAsync()
				return nil, nil
			}
			return s.ScanNow()
		})})

	add(capreg.Cap{ID: "library.remove", Summary: "forget a media folder; its files and its records in it are left as they are", Writes: true,
		Params: []capreg.Param{{Name: "folder", Kind: capreg.Path, Required: true, Positional: true, Doc: "the folder"}},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			return nil, s.lib.RemoveFolder(a.String("folder"))
		})})

	add(capreg.Cap{ID: "library.scan", Summary: "look for new, changed and missing files in every folder", Writes: true,
		Params: []capreg.Param{{Name: "wait", Kind: capreg.Bool, Default: true, Doc: "wait for the scan (false: scan in the background; library list shows scanning)"}},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			if !a.Bool("wait") {
				s.ScanAsync()
				return nil, nil
			}
			return s.ScanNow()
		})})

	// ---- one video ----

	add(capreg.Cap{ID: "entry.info", Summary: "one video: its names, file facts, progress, choices and version",
		Params: []capreg.Param{entry},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			e, err := s.resolve(a.String("entry"))
			if err != nil {
				return nil, err
			}
			return struct {
				library.Entry
				Version string `json:"version"`
			}{e, e.Version()}, nil
		})})

	add(capreg.Cap{ID: "entry.episodes", Summary: "the episodes of a video's series, in order, with progress",
		Params: []capreg.Param{entry},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			e, err := s.resolve(a.String("entry"))
			if err != nil {
				return nil, err
			}
			return s.EpisodeList(e), nil
		})})

	add(capreg.Cap{ID: "entry.watched", Summary: "mark a video watched or not (its position goes back to the start)", Writes: true,
		Params: []capreg.Param{entry, {Name: "watched", Kind: capreg.Bool, Default: true, Doc: "false to mark it not watched"}, base},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			e, err := s.resolve(a.String("entry"))
			if err != nil {
				return nil, err
			}
			w := a.Bool("watched")
			return s.progressResult(e.ID, s.lib.UpdateIf(e.ID, a.String("base"), func(e *library.Entry) { e.Watched = w; e.Position = 0 }))
		})})

	// ---- progress ----

	add(capreg.Cap{ID: "progress.get", Summary: "where a video was left, whether it was watched, and its version",
		Params: []capreg.Param{entry},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			e, err := s.resolve(a.String("entry"))
			if err != nil {
				return nil, err
			}
			return progressOf(e), nil
		})})

	add(capreg.Cap{ID: "progress.set", Summary: "set where a video was left, as the player does while playing (within the last 5 % counts as watched)", Writes: true,
		Params: []capreg.Param{entry,
			{Name: "position", Kind: capreg.Number, Required: true, Doc: "seconds, or [h:]m:s"},
			{Name: "duration", Kind: capreg.Number, Doc: "the video's length (default: as probed)"},
			base},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			e, err := s.resolve(a.String("entry"))
			if err != nil {
				return nil, err
			}
			pos, dur := a.Float("position"), a.Float("duration")
			if pos < 0 {
				return nil, capreg.Usagef("progress set: position must not be negative")
			}
			if !a.Has("duration") && e.Probe != nil {
				dur = e.Probe.Duration
			}
			return s.progressResult(e.ID, s.lib.UpdateIf(e.ID, a.String("base"), func(x *library.Entry) { setProgress(x, pos, dur, time.Now()) }))
		})})

	// ---- comments ----

	add(capreg.Cap{ID: "comments.info", Summary: "where a video's comments come from, how many there are, and any problem found",
		Params: []capreg.Param{entry},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			sess, err := s.sessionOf(a.String("entry"))
			if err != nil {
				return nil, err
			}
			return sess.Comments, nil
		})})

	add(capreg.Cap{ID: "comments.source", Summary: "use this comment file for a video (.json/.xml, snapshot folder or raw ZIP); without -file, go back to the automatic order", Writes: true,
		Params: []capreg.Param{entry, {Name: "file", Kind: capreg.Path, Doc: "the comment file"}},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			e, err := s.resolve(a.String("entry"))
			if err != nil {
				return nil, err
			}
			return s.SetCommentSource(e.ID, a.String("file"))
		})})

	add(capreg.Cap{ID: "comments.offset", Summary: "shift a video's comments by this many milliseconds; without -ms, back to the one the comment data records", Writes: true,
		Params: []capreg.Param{entry, {Name: "ms", Kind: capreg.Int, Doc: "-600000 … 600000; positive shows them later"}},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			e, err := s.resolve(a.String("entry"))
			if err != nil {
				return nil, err
			}
			var ms *int64
			if a.Has("ms") {
				v := a.Int("ms")
				ms = &v
			}
			return nil, s.SetOffset(e.ID, ms)
		})})

	add(capreg.Cap{ID: "comments.embed", Summary: "put the same-name .json's comments into the MKV itself (in place, verified; the comments it replaces are kept in the folder's records)",
		Writes: true, Confirm: true,
		Params: []capreg.Param{entry},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			e, err := s.resolve(a.String("entry"))
			if err != nil {
				return nil, err
			}
			if !a.Confirmed() {
				return nil, capreg.NeedConfirm("comments embed rewrites the video file; add -yes", map[string]any{"rewrite": e.Path})
			}
			return s.Embed(e.ID)
		})})

	add(capreg.Cap{ID: "comments.filter", Summary: "which of a video's comments the player shows (positions per thread) and what each rule hid; -settings overrides the saved settings",
		Params: []capreg.Param{entry, {Name: "settings", Kind: capreg.String, Doc: `a settings document (JSON) to filter by, e.g. {"filters":{"ngWords":["x"]}}; default: the saved one`}},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			sess, err := s.sessionOf(a.String("entry"))
			if err != nil {
				return nil, err
			}
			doc := s.settings.Get()
			if a.Has("settings") {
				doc = []byte(a.String("settings"))
			}
			return s.FilterComments(sess, doc)
		})})

	add(capreg.Cap{ID: "comments.list", Summary: "a video's comments as the player shows them (after the filters), in time order; -from/-to keep a stretch, -all skips the filters",
		Params: []capreg.Param{entry,
			{Name: "from", Kind: capreg.Number, Doc: "from this time (seconds or [h:]m:s)"},
			{Name: "to", Kind: capreg.Number, Doc: "up to this time"},
			{Name: "all", Kind: capreg.Bool, Doc: "every comment in the data, not only those shown"},
			{Name: "limit", Kind: capreg.Int, Doc: "at most this many (0: all)"}},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			sess, err := s.sessionOf(a.String("entry"))
			if err != nil {
				return nil, err
			}
			return s.ListComments(sess, a.Bool("all"), a.Float("from"), a.Float("to"), int(a.Int("limit")))
		}),
		Text: printComments})

	add(capreg.Cap{ID: "comments.translate", Summary: "translate the comments a video shows (as the player does while playing) with the AI component; waits until done, keeps the result in the folder's records",
		Writes: true,
		Params: []capreg.Param{entry, {Name: "to", Kind: capreg.String, Required: true, Enum: []string{"zh", "zh-Hant", "ja", "en", "ko"}, Doc: "the language to translate into"}},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			e, err := s.resolve(a.String("entry"))
			if err != nil {
				return nil, err
			}
			sess, err := s.Session(e.ID)
			if err != nil {
				return nil, err
			}
			if sess.Comments.playback == nil {
				return nil, capreg.NotFoundf("no comments for this video")
			}
			return s.TranslateComments(ctx, sess, e, a.String("to"))
		})})

	add(capreg.Cap{ID: "analysis.get", Summary: "the comment analysis of a video: hot moments, topics, frequent comments (the report's data)",
		Params: []capreg.Param{entry},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			sess, err := s.sessionOf(a.String("entry"))
			if err != nil {
				return nil, err
			}
			if sess.Comments.playback == nil {
				return nil, capreg.NotFoundf("no comments for this video")
			}
			b, err := s.analysis(sess)
			if err != nil {
				return nil, err
			}
			return json.RawMessage(b), nil
		})})

	// ---- subtitles ----

	add(capreg.Cap{ID: "subs.tracks", Summary: "the subtitles a video has (files beside it, its own tracks, AI) and which are chosen",
		Params: []capreg.Param{entry},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			e, err := s.resolve(a.String("entry"))
			if err != nil {
				return nil, err
			}
			return s.SubtitleTracks(e), nil
		})})

	add(capreg.Cap{ID: "subs.show", Summary: "a video's subtitles as timed text: the chosen track, or -track; -from/-to keep a stretch",
		Params: []capreg.Param{entry,
			{Name: "track", Kind: capreg.String, Doc: "file:<name>, manual, mkv:<n>, ai:src or ai:tr (default: the chosen one; subs tracks lists them)"},
			{Name: "lang", Kind: capreg.String, Doc: "for ai:tr, the translation's language (zh, zh-Hant, ja, en, ko)"},
			{Name: "from", Kind: capreg.Number, Doc: "from this time (seconds or [h:]m:s)"},
			{Name: "to", Kind: capreg.Number, Doc: "up to this time"}},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			e, err := s.resolve(a.String("entry"))
			if err != nil {
				return nil, err
			}
			key := a.String("track")
			if key == "" {
				key = e.SubPick
			}
			if key == "" || key == "off" {
				return nil, capreg.Usagef("no subtitle track chosen for this video: give -track (subs tracks lists them)")
			}
			lines, err := s.SubtitleLines(e, key, a.String("lang"))
			if err != nil {
				return nil, err
			}
			return map[string]any{"track": key, "lines": subs.Between(lines, a.Float("from"), a.Float("to"))}, nil
		}),
		Text: printLines})

	add(capreg.Cap{ID: "subs.pick", Summary: "choose a video's subtitles: -pick (and -pick2 for a second line) take file:<name>, manual, mkv:<n>, ai:src, ai:tr or off", Writes: true,
		Params: []capreg.Param{entry,
			{Name: "pick", Kind: capreg.String, Doc: "the first track"},
			{Name: "pick2", Kind: capreg.String, Doc: "the second track"},
			{Name: "file", Kind: capreg.Path, Doc: "a subtitle file chosen by hand (.srt, .ass, .ssa, .vtt), then used as manual"}},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			e, err := s.resolve(a.String("entry"))
			if err != nil {
				return nil, err
			}
			pick, pick2 := e.SubPick, e.SubPick2
			if a.Has("pick") {
				pick = a.String("pick")
			}
			if a.Has("pick2") {
				pick2 = a.String("pick2")
			}
			return nil, s.PickSubtitles(e.ID, pick, pick2, a.String("file"))
		})})

	// ---- settings ----

	add(capreg.Cap{ID: "settings.get", Summary: "the player's settings document (comment display, filters, subtitles …)",
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			return json.RawMessage(s.settings.Get()), nil
		})})

	add(capreg.Cap{ID: "settings.set", Summary: "change settings: -patch is a JSON merge patch (RFC 7386) applied to the document; -document replaces it whole", Writes: true,
		Params: []capreg.Param{
			{Name: "patch", Kind: capreg.String, Doc: `e.g. {"comments":{"opacity":0.8}}; null removes a key`},
			{Name: "document", Kind: capreg.String, Doc: "the whole settings document, as JSON (what the player saves)"}},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			if a.Has("patch") == a.Has("document") {
				return nil, capreg.Usagef("settings set: give -patch or -document")
			}
			var b []byte
			if a.Has("document") {
				b = []byte(a.String("document"))
			} else {
				var p any
				if err := json.Unmarshal([]byte(a.String("patch")), &p); err != nil {
					return nil, capreg.Usagef("settings set: -patch is not JSON: %v", err)
				}
				var doc any
				json.Unmarshal(s.settings.Get(), &doc)
				var err error
				if b, err = json.Marshal(mergePatch(doc, p)); err != nil {
					return nil, err
				}
			}
			if err := s.settings.Put(b); err != nil {
				return nil, capreg.Usagef("%v", err)
			}
			return json.RawMessage(b), nil
		})})

	add(capreg.Cap{ID: "prefs.get", Summary: "the interface choices: language, theme, library order",
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			return s.prefs.get(), nil
		})})

	add(capreg.Cap{ID: "prefs.set", Summary: "change interface choices", Writes: true,
		Params: []capreg.Param{
			{Name: "lang", Kind: capreg.String, Enum: prefChoices["lang"], Doc: "interface language"},
			{Name: "theme", Kind: capreg.String, Enum: prefChoices["theme"], Doc: "colour theme"},
			{Name: "sort", Kind: capreg.String, Enum: prefChoices["sort"], Doc: "library order"},
		},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			ch := map[string]string{}
			for _, k := range []string{"lang", "theme", "sort"} {
				if a.Has(k) {
					ch[k] = a.String(k)
				}
			}
			b, _ := json.Marshal(ch)
			p, err := s.prefs.update(b)
			if err != nil {
				return nil, capreg.Usagef("%v", err)
			}
			return p, nil
		})})

	// ---- AI component ----

	add(capreg.Cap{ID: "ai.status", Summary: "the optional AI component: what is installed, its configuration (without keys), the download",
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			e := s.aiEngine()
			if e == nil {
				return nil, capreg.NotFoundf("no AI support in this build")
			}
			out := s.aiStatus()
			out["config"] = e.Config()
			out["download"] = e.Download()
			out["recommended"] = ai.Recommended
			return out, nil
		})})

	add(capreg.Cap{ID: "ai.config", Summary: "change the AI configuration: -config is a JSON object like ai status's config", Writes: true,
		Params: []capreg.Param{{Name: "config", Kind: capreg.String, Required: true, Doc: "the whole configuration, as JSON"}},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			e := s.aiEngine()
			if e == nil {
				return nil, capreg.NotFoundf("no AI support in this build")
			}
			var c ai.PublicConfig
			if err := json.Unmarshal([]byte(a.String("config")), &c); err != nil {
				return nil, capreg.Usagef("ai config: %v", err)
			}
			if err := e.SetConfig(c); err != nil {
				return nil, err
			}
			s.logf("ai: configuration changed (recognition %s, translation %s)", c.ASR.Kind, c.MT.Kind)
			return nil, nil
		})})

	add(capreg.Cap{ID: "ai.download", Summary: "download the recommended models and runtime (goes online; pinned SHA-256); returns at once, ai status shows progress",
		Writes: true, Confirm: true,
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			e := s.aiEngine()
			if e == nil {
				return nil, capreg.NotFoundf("no AI support in this build")
			}
			if !a.Confirmed() {
				return nil, capreg.NeedConfirm("ai download goes online and fetches the recommended files; add -yes", ai.Recommended)
			}
			if err := e.StartDownload(); err != nil {
				return nil, err
			}
			s.logf("ai: download of the recommended models started")
			return e.Download(), nil
		})})

	add(capreg.Cap{ID: "ai.cancel", Summary: "stop the AI download",
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			if e := s.aiEngine(); e != nil {
				e.CancelDownload()
			}
			return nil, nil
		})})

	// ---- a folder's own tools (skills.go) ----

	add(capreg.Cap{ID: "skills.list", Summary: "the media folders' own tools (<folder>/.jusplay/skills/<name>: SKILL.md, and skill.json to run it); results go to .jusplay/out/<name>",
		Params: []capreg.Param{{Name: "folder", Kind: capreg.Path, Doc: "only this added folder's"}},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			return s.Skills(a.String("folder"))
		})})

	add(capreg.Cap{ID: "skills.run", Summary: "run a media folder's skill (its skill.json command, in the folder); the first time, and after its files change, it needs -yes: without, says what it would run",
		Writes: true, Confirm: true,
		Params: []capreg.Param{{Name: "name", Kind: capreg.String, Required: true, Positional: true, Doc: "the skill (skills list)"},
			{Name: "folder", Kind: capreg.Path, Doc: "the added folder it is in (needed when several have one of that name)"},
			{Name: "args", Kind: capreg.Strings, Doc: "more arguments for its command"}},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			return s.RunSkill(ctx, a.String("name"), a.String("folder"), a.Confirmed(), a.Strings("args"))
		})})

	// ---- links (link.go) ----

	add(capreg.Cap{ID: "note.add", Summary: "write a line about a moment into a Jusnote notebook (记一笔): the date, a link back to the moment, and -text; by default the video and moment the window plays now, and the notebook chosen in the window (settings notes.notebook, notes.file)", Writes: true,
		Params: []capreg.Param{
			{Name: "text", Kind: capreg.String, Required: true, Stdin: true, Doc: "what to write (one line)"},
			{Name: "entry", Kind: capreg.Ref, Doc: "the video (default: the one playing in the window); a jus://play link carries its moment"},
			{Name: "at", Kind: capreg.Number, Doc: "the moment: seconds, or [h:]m:s (default: where it plays, or the link's)"},
			{Name: "notebook", Kind: capreg.Path, Doc: "the Jusnote notebook folder (default: the one chosen in the window)"},
			{Name: "file", Kind: capreg.String, Doc: "the note in it (default: " + DefaultNoteFile + ")"}},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			var at *float64
			if a.Has("at") {
				t := a.Float("at")
				at = &t
			}
			return s.AddNote(ctx, a.String("entry"), at, a.String("text"), a.String("notebook"), a.String("file"))
		})})

	add(capreg.Cap{ID: "link.info", Summary: "what a jus://play link points to, for another app to show it (a note's link preview): its title, the moment, and how far it was watched",
		Params: []capreg.Param{{Name: "link", Kind: capreg.String, Required: true, Positional: true, Doc: "a jus://play link"}},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			link := a.String("link")
			if !strings.HasPrefix(link, linkPrefix) {
				return nil, capreg.Usagef("not a jus://play link: %s", link)
			}
			e, err := s.resolve(link)
			if err != nil {
				return nil, err
			}
			t, _ := linkMoment(link)
			out := map[string]any{"title": linkTitle(e, 0), "at": t, "watched": e.Watched, "missing": e.Missing}
			if e.Probe != nil && e.Probe.Duration > 0 {
				out["duration"] = e.Probe.Duration
				out["progress"] = e.Position / e.Probe.Duration
			}
			return out, nil
		})})

	add(capreg.Cap{ID: "link.make", Summary: "a jus://play link to a video (at a moment with -at), and the same as a Markdown link for a note",
		Params: []capreg.Param{entry, {Name: "at", Kind: capreg.Number, Doc: "the moment: seconds, or [h:]m:s"}},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			e, err := s.resolve(a.String("entry"))
			if err != nil {
				return nil, err
			}
			t := a.Float("at")
			if !a.Has("at") {
				t, _ = linkMoment(a.String("entry"))
			}
			link := s.LinkOf(e, t)
			return map[string]any{"link": link, "markdown": markdownLink(linkTitle(e, t), link), "portable": !strings.HasPrefix(link, linkPrefix+"file/")}, nil
		}),
		Text: func(w io.Writer, v any) { fmt.Fprintln(w, v.(map[string]any)["link"]) }})

	// ---- the change log (changes.go) ----

	r.Add(capreg.Cap{ID: "changes.list", Summary: "the changes made through Jusplay, newest first: what, when, and who (window, command line, or an agent's harness and session)",
		Params: []capreg.Param{
			{Name: "session", Kind: capreg.String, Doc: "only this session's (the -run / JUS_RUN it was made with)"},
			{Name: "by", Kind: capreg.String, Doc: "only this harness's (the -harness / JUS_HARNESS it was made with)"},
			{Name: "limit", Kind: capreg.Int, Default: int64(50), Doc: "at most this many (0: all)"}},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			all := s.Changes()
			out := []Change{}
			for i := len(all) - 1; i >= 0; i-- {
				c := all[i]
				if a.Has("session") && c.Source.Run != a.String("session") || a.Has("by") && c.Source.Harness != a.String("by") {
					continue
				}
				out = append(out, c)
				if n := a.Int("limit"); n > 0 && int64(len(out)) >= n {
					break
				}
			}
			return out, nil
		})})

	r.Add(capreg.Cap{ID: "changes.revert", Summary: "undo one session's changes (newest first), each only if nothing changed it since; without -yes, says what it would do",
		Confirm: true,
		Params:  []capreg.Param{{Name: "session", Kind: capreg.String, Required: true, Doc: "the session id (as changes list shows it: source.run)"}},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			if !a.Confirmed() {
				plan, err := s.Revert(ctx, a.String("session"), false)
				if err != nil {
					return nil, err
				}
				return nil, capreg.NeedConfirm("changes revert puts these back; add -yes", plan)
			}
			return s.Revert(ctx, a.String("session"), true)
		})})

	registerLive(r, with, get, entry)
	registerAgents(r)
}

// resolve finds the entry a reference names: an id, or a video's path (a
// file outside the library is added the way opening it does).
func (s *Server) resolve(ref string) (library.Entry, error) {
	if ref == "" {
		return library.Entry{}, capreg.Usagef("no video given")
	}
	if strings.HasPrefix(ref, "jus://") {
		e, _, err := s.resolveLink(ref)
		return e, err
	}
	s.lib.Refresh()
	if e, ok := s.lib.Get(ref); ok {
		return e, nil
	}
	if filepath.IsAbs(ref) {
		if e, ok := s.lib.Get(library.IDFor(ref)); ok {
			return e, nil
		}
		if st, err := os.Stat(ref); err == nil && !st.IsDir() {
			return s.lib.Ensure(ref)
		}
	}
	return library.Entry{}, capreg.NotFoundf("no video %q (give its path, or an id from library list)", ref)
}

func (s *Server) sessionOf(ref string) (*Session, error) {
	e, err := s.resolve(ref)
	if err != nil {
		return nil, err
	}
	return s.Session(e.ID)
}

func (s *Server) aiEngine() *ai.Engine {
	s.ai.mu.Lock()
	defer s.ai.mu.Unlock()
	return s.ai.e
}

// setProgress is what the player records while playing.
func setProgress(e *library.Entry, pos, dur float64, now time.Time) {
	e.Position, e.LastPlayed = pos, &now
	// Within the last 5 % (credits) counts as watched.
	if dur > 0 && pos >= dur*0.95 {
		e.Watched, e.Position = true, 0
	}
}

type progress struct {
	ID         string     `json:"id"`
	Path       string     `json:"path"`
	Position   float64    `json:"position"`
	Duration   float64    `json:"duration"`
	Watched    bool       `json:"watched"`
	LastPlayed *time.Time `json:"lastPlayed,omitempty"`
	Version    string     `json:"version"`
}

func progressOf(e library.Entry) progress {
	p := progress{ID: e.ID, Path: e.Path, Position: e.Position, Watched: e.Watched, LastPlayed: e.LastPlayed, Version: e.Version()}
	if e.Probe != nil {
		p.Duration = e.Probe.Duration
	}
	return p
}

func (s *Server) progressResult(id string, err error) (any, error) {
	if errors.Is(err, library.ErrConflict) {
		return nil, capreg.Conflictf("the video's progress changed since that version (progress get shows it now)")
	}
	if err != nil {
		return nil, err
	}
	e, _ := s.lib.Get(id)
	return progressOf(e), nil
}

// listRow is one file in library list.
type listRow struct {
	ID         string     `json:"id"`
	Path       string     `json:"path"`
	Folder     string     `json:"folder,omitempty"`
	Series     string     `json:"series"`
	Season     *int       `json:"season,omitempty"`
	Episode    *float64   `json:"episode,omitempty"`
	Title      string     `json:"title,omitempty"`
	Duration   float64    `json:"duration,omitempty"`
	Position   float64    `json:"position"`
	Watched    bool       `json:"watched"`
	LastPlayed *time.Time `json:"lastPlayed,omitempty"`
	Comments   *int       `json:"comments,omitempty"`
	Missing    bool       `json:"missing,omitempty"`
	// AltOf: another version of that file's episode.
	AltOf   string `json:"altOf,omitempty"`
	Version string `json:"version"`
}

func rowOf(e LibraryEntry) listRow {
	r := listRow{ID: e.ID, Path: e.Path, Folder: e.Folder, Series: e.Series, Season: e.Season, Episode: e.Episode, Title: e.Title,
		Position: e.Position, Watched: e.Watched, LastPlayed: e.LastPlayed, Comments: e.CommentCount, Missing: e.Missing, AltOf: e.AltOf, Version: e.Version()}
	if e.Probe != nil {
		r.Duration = e.Probe.Duration
	}
	return r
}

func printList(w io.Writer, v any) {
	m, _ := v.(map[string]any)
	rows, _ := m["entries"].([]any)
	series := ""
	for _, x := range rows {
		r, _ := x.(map[string]any)
		if s, _ := r["series"].(string); s != series {
			series = s
			fmt.Fprintf(w, "%s\n", s)
		}
		mark := " "
		if w, _ := r["watched"].(bool); w {
			mark = "✓"
		} else if p, _ := r["position"].(json.Number); p.String() != "0" && p != "" {
			mark = "…"
		}
		fmt.Fprintf(w, "  %s %s  %s\n", mark, r["id"], filepath.Base(fmt.Sprint(r["path"])))
	}
	fmt.Fprintf(w, "%d files\n", len(rows))
}

// SetCommentSource makes path the video's comment file ("" for the
// automatic order); a file that fails verification is refused.
func (s *Server) SetCommentSource(id, path string) (Comments, error) {
	e, ok := s.lib.Get(id)
	if !ok {
		return Comments{}, capreg.NotFoundf("no such entry")
	}
	c := LoadComments(e.Path, path)
	if path != "" && c.Source != "manual" {
		return c, errors.New(c.Error)
	}
	if err := s.lib.Update(id, func(e *library.Entry) { e.Comments = path }); err != nil {
		return c, err
	}
	s.forget(id) // reopened with the new source on the next request
	return c, nil
}

// SetOffset keeps the comment offset chosen for a file (nil: the one the
// comment data records).
func (s *Server) SetOffset(id string, ms *int64) error {
	if ms != nil && (*ms < -600000 || *ms > 600000) {
		return capreg.Usagef("offset out of range")
	}
	if err := s.lib.Update(id, func(e *library.Entry) { e.OffsetMs = ms }); err != nil {
		return capreg.NotFoundf("%v", err)
	}
	s.mu.Lock()
	if sess := s.sessions[id]; sess != nil {
		sess.OffsetMs = ms
	}
	s.mu.Unlock()
	return nil
}

// PickSubtitles keeps the subtitle choice of a file; file, if set, is a
// subtitle file chosen by hand.
func (s *Server) PickSubtitles(id, pick, pick2, file string) error {
	if file != "" {
		if _, ok := subs.Formats[strings.ToLower(filepath.Ext(file))]; !ok || !filepath.IsAbs(file) {
			return capreg.Usagef("not a subtitle file (.srt, .ass, .ssa, .vtt)")
		}
		if _, err := os.Stat(file); err != nil {
			return err
		}
	}
	err := s.lib.Update(id, func(e *library.Entry) {
		e.SubPick, e.SubPick2 = pick, pick2
		if file != "" {
			e.SubFile = file
		}
	})
	if err != nil {
		return capreg.NotFoundf("%v", err)
	}
	return nil
}

// mergePatch applies an RFC 7386 JSON merge patch.
func mergePatch(doc, patch any) any {
	p, ok := patch.(map[string]any)
	if !ok {
		return patch
	}
	d, ok := doc.(map[string]any)
	if !ok {
		d = map[string]any{}
	}
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if p[k] == nil {
			delete(d, k)
			continue
		}
		d[k] = mergePatch(d[k], p[k])
	}
	return d
}

func printLines(w io.Writer, v any) {
	m, _ := v.(map[string]any)
	ls, _ := m["lines"].([]any)
	for _, x := range ls {
		l, _ := x.(map[string]any)
		st, _ := l["start"].(json.Number).Float64()
		fmt.Fprintf(w, "%s  %s\n", clock(st), strings.ReplaceAll(fmt.Sprint(l["text"]), "\n", " / "))
	}
}

// clock is t as [h:]mm:ss.
func clock(t float64) string {
	s := int(t)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func printComments(w io.Writer, v any) {
	m, _ := v.(map[string]any)
	cs, _ := m["comments"].([]any)
	for _, x := range cs {
		c, _ := x.(map[string]any)
		t, _ := c["time"].(json.Number).Float64()
		fmt.Fprintf(w, "%s  %s\n", clock(t), strings.ReplaceAll(fmt.Sprint(c["body"]), "\n", " / "))
	}
	fmt.Fprintf(w, "%v shown of %v\n", m["shown"], m["total"])
}
