package main

// The file tools — comment snapshots (import, validate, derive, pack) and
// the comments inside an MKV (attach, embed, verify, extract) — as
// capabilities, like everything else: in help -json and the agent guide,
// with -json results. The spellings of before (import-xml, embed, …) still
// work (legacyCommand).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tieumi221E/Jus/capreg"
	"github.com/Tieumi221E/Jusplay/internal/bundle"
	"github.com/Tieumi221E/Jusplay/internal/comments"
	"github.com/Tieumi221E/Jusplay/internal/derive"
	"github.com/Tieumi221E/Jusplay/internal/detzip"
	"github.com/Tieumi221E/Jusplay/internal/embed"
	"github.com/Tieumi221E/Jusplay/internal/importxml"
	"github.com/Tieumi221E/Jusplay/internal/importzouryou"
	"github.com/Tieumi221E/Jusplay/internal/library"
	"github.com/Tieumi221E/Jusplay/internal/mkv"
	"github.com/Tieumi221E/Jusplay/internal/snapshot"
)

// SnapshotSummary is what validate says about a snapshot.
type SnapshotSummary struct {
	VideoID    string `json:"videoId"`
	SnapshotID string `json:"snapshotId"`
	Responses  int    `json:"responses"`
	Origin     string `json:"origin"`
	Status     string `json:"status"`
}

// EmbedResult is what mkv embed did.
type EmbedResult struct {
	Video        string `json:"video"`
	Embedded     bool   `json:"embedded"`
	Reason       string `json:"reason,omitempty"` // why not (no comment file)
	Source       string `json:"source,omitempty"`
	Comments     int    `json:"comments"`
	Replaced     bool   `json:"replaced"`
	Backup       string `json:"backup,omitempty"`
	Recovered    string `json:"recovered,omitempty"`
	OffsetMs     int64  `json:"offsetMs"`
	SyncVerified bool   `json:"syncVerified"`
	Packets      int    `json:"packets"`
	Ms           int64  `json:"ms"`
}

func registerTools(r *capreg.Registry) {
	snap := capreg.Param{Name: "snapshot", Kind: capreg.Path, Required: true, Positional: true, Doc: "a snapshot folder or its raw ZIP"}
	video := capreg.Param{Name: "video", Kind: capreg.Path, Required: true, Positional: true, Doc: "the MKV"}
	videoID := func(required bool) capreg.Param {
		return capreg.Param{Name: "video-id", Kind: capreg.String, Required: required, Doc: "the Niconico video id, e.g. so00000001"}
	}
	out := func(doc string) capreg.Param {
		return capreg.Param{Name: "o", Kind: capreg.Path, Required: true, Doc: doc}
	}
	forkList := capreg.Param{Name: "forks", Kind: capreg.String, Doc: "comma-separated forks (default all)"}

	importer := func(id, what, status string, run func([]byte, string, string, string, string) (*snapshot.Capture, error)) {
		r.Add(capreg.Cap{ID: id, Summary: "make a comment snapshot folder from " + what, Writes: true,
			Params: []capreg.Param{{Name: "file", Kind: capreg.Path, Required: true, Positional: true, Doc: "the file to import"}, videoID(true), out("the new snapshot folder")},
			Run: func(ctx context.Context, a capreg.Args) (any, error) {
				b, err := os.ReadFile(a.String("file"))
				if err != nil {
					return nil, err
				}
				c, err := run(b, a.String("video-id"), filepath.Base(a.String("file")), version, a.String("o"))
				if err != nil {
					return nil, err
				}
				return map[string]any{"snapshot": c.SnapshotID, "out": a.String("o"), "status": "partial", "note": status}, nil
			},
			Text: func(w io.Writer, v any) {
				m := v.(map[string]any)
				fmt.Fprintf(w, "snapshot %v written to %v (status partial: %v)\n", m["snapshot"], m["out"], m["note"])
			}})
	}
	importer("snapshot.import-xml", "a legacy Niconico comment XML", "legacy XML", importxml.Import)
	importer("snapshot.import-zouryou", "a コメント増量 JSON export", "コメント増量 export", importzouryou.Import)

	r.Add(capreg.Cap{ID: "snapshot.validate", Summary: "check a comment snapshot (folder or raw ZIP) against its schema and hashes",
		Params: []capreg.Param{snap},
		Run: func(ctx context.Context, a capreg.Args) (any, error) {
			s, err := load(a.String("snapshot"))
			if err != nil {
				return nil, err
			}
			return SnapshotSummary{s.Capture.VideoID, s.Capture.SnapshotID, len(s.Capture.Responses), string(s.Capture.Origin), string(s.Report.Status)}, nil
		},
		Text: func(w io.Writer, v any) {
			s := capreg.As[SnapshotSummary](v)
			fmt.Fprintf(w, "ok: %s %s, %d response(s), origin %s, status %s\n", s.VideoID, s.SnapshotID, s.Responses, s.Origin, s.Status)
		}})

	r.Add(capreg.Cap{ID: "snapshot.derive", Summary: "the playback JSON the player draws, derived from a snapshot, and a report of what was merged or dropped", Writes: true,
		Params: []capreg.Param{snap, out("the playback JSON (never overwritten)"),
			{Name: "report", Kind: capreg.Path, Doc: "also write the derivation report here"}, forkList},
		Run: func(ctx context.Context, a capreg.Args) (any, error) {
			s, err := load(a.String("snapshot"))
			if err != nil {
				return nil, err
			}
			playback, rep, err := derive.Derive(s, derive.Options{Forks: forks(a.String("forks"))})
			if err != nil {
				return nil, err
			}
			if err := writeNew(a.String("o"), playback); err != nil {
				return nil, err
			}
			if p := a.String("report"); p != "" {
				rb, err := snapshot.MarshalDoc(rep)
				if err != nil {
					return nil, err
				}
				if err := writeNew(p, rb); err != nil {
					return nil, err
				}
			}
			return map[string]any{"out": a.String("o"), "report": rep}, nil
		},
		Text: func(w io.Writer, v any) {
			summarize(w, capreg.As[struct{ Report derive.Report }](v).Report)
		}})

	r.Add(capreg.Cap{ID: "snapshot.pack", Summary: "a snapshot folder as its deterministic raw ZIP", Writes: true,
		Params: []capreg.Param{{Name: "snapshot", Kind: capreg.Path, Required: true, Positional: true, Doc: "the snapshot folder"}, out("the ZIP (never overwritten)")},
		Run: func(ctx context.Context, a capreg.Args) (any, error) {
			s, err := load(a.String("snapshot"))
			if err != nil {
				return nil, err
			}
			z, err := detzip.Write(s.Files())
			if err != nil {
				return nil, err
			}
			if err := writeNew(a.String("o"), z); err != nil {
				return nil, err
			}
			return map[string]any{"out": a.String("o"), "bytes": len(z), "sha256": snapshot.SHA256Hex(z)}, nil
		},
		Text: func(w io.Writer, v any) {
			m := v.(map[string]any)
			fmt.Fprintf(w, "%v: %v bytes, sha256 %v\n", m["out"], m["bytes"], m["sha256"])
		}})

	r.Add(capreg.Cap{ID: "mkv.attach", Summary: "a new MKV: the video with a snapshot's comments attached (the original is not touched)", Writes: true,
		Params: []capreg.Param{video, snap, out("the new MKV (never overwritten)"), forkList,
			{Name: "offset-ms", Kind: capreg.Int, Doc: "sync offset: nicoMs = localMs + offset"},
			{Name: "renderer-mode", Kind: capreg.String, Default: "default", Doc: "niconicomments mode"}},
		Run: func(ctx context.Context, a capreg.Args) (any, error) {
			s, err := load(a.String("snapshot"))
			if err != nil {
				return nil, err
			}
			probe, err := mkv.ProbeFile(a.String("video"))
			if err != nil {
				return nil, err
			}
			atts, rep, err := bundle.Build(s, probe, bundle.Options{
				Derive:    derive.Options{Forks: forks(a.String("forks"))},
				OffsetMs:  a.Int("offset-ms"),
				Renderer:  bundle.Renderer{Name: "niconicomments", Version: "0.4.1-jusplay", Mode: a.String("renderer-mode"), Fonts: "Jus Sans 2.005"},
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
			})
			if err != nil {
				return nil, err
			}
			start := time.Now()
			ver, err := mkv.Attach(a.String("video"), a.String("o"), atts)
			if err != nil {
				return nil, err
			}
			return map[string]any{"out": a.String("o"), "ms": time.Since(start).Milliseconds(), "report": rep, "verification": ver}, nil
		},
		Text: func(w io.Writer, v any) {
			m := capreg.As[struct {
				Out          string
				Ms           int64
				Report       derive.Report
				Verification any
			}](v)
			summarize(w, m.Report)
			fmt.Fprintf(w, "wrote %s in %s\n", m.Out, time.Duration(m.Ms)*time.Millisecond)
			b, _ := json.MarshalIndent(m.Verification, "", "  ")
			fmt.Fprintln(w, string(b))
		}})

	r.Add(capreg.Cap{ID: "mkv.embed", Summary: "put comments into an MKV in place: the same-name .json, or the comment file given (verified packet by packet before the swap; comments it replaces are kept in the folder's records); a video without a comment file is left as it is (embedded: false)", Writes: true,
		Params: []capreg.Param{video,
			{Name: "comments", Kind: capreg.Path, Positional: true, Doc: "a comment JSON, snapshot folder, raw ZIP or XML (default: the same-name .json)"},
			videoID(false),
			{Name: "offset-ms", Kind: capreg.Int, Doc: "sync offset (nicoMs = localMs + offset); default keeps the embedded one"},
			{Name: "verified", Kind: capreg.Bool, Doc: "with -offset-ms: the offset was checked against the picture"}},
		Run: func(ctx context.Context, a capreg.Args) (any, error) {
			return embedFile(a.String("video"), a.String("comments"), a.String("video-id"), a.Has("offset-ms"), a.Int("offset-ms"), a.Bool("verified"))
		},
		Text: func(w io.Writer, v any) { printEmbed(w, capreg.As[EmbedResult](v)) }})

	r.Add(capreg.Cap{ID: "mkv.verify", Summary: "check the comments inside an MKV: every attachment consistent, and the playback re-derives identically",
		Params: []capreg.Param{video},
		Run: func(ctx context.Context, a capreg.Args) (any, error) {
			files, err := mkv.Extract(a.String("video"))
			if err != nil {
				return nil, err
			}
			if len(files) == 0 {
				return nil, capreg.NotFoundf("no jusplay attachments in %s", filepath.Base(a.String("video")))
			}
			m, err := bundle.Check(files)
			if err != nil {
				return nil, err
			}
			return map[string]any{"videoId": m.VideoID, "snapshotId": m.SnapshotID, "status": m.CaptureStatus, "attachments": len(files)}, nil
		},
		Text: func(w io.Writer, v any) {
			m := v.(map[string]any)
			fmt.Fprintf(w, "ok: %v snapshot %v, status %v, %v attachments consistent, playback re-derives identically\n", m["videoId"], m["snapshotId"], m["status"], m["attachments"])
		}})

	r.Add(capreg.Cap{ID: "mkv.extract", Summary: "take the comments out of an MKV into a new folder (only when they verify)", Writes: true,
		Params: []capreg.Param{video, out("the new folder (must not exist)")},
		Run: func(ctx context.Context, a capreg.Args) (any, error) {
			dir := a.String("o")
			if _, err := os.Stat(dir); err == nil {
				return nil, capreg.Usagef("%s already exists", dir)
			}
			files, err := mkv.Extract(a.String("video"))
			if err != nil {
				return nil, err
			}
			if _, err := bundle.Check(files); err != nil {
				return nil, fmt.Errorf("attachments failed verification, not extracted: %w", err)
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}
			var names []string
			for name, b := range files {
				if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
					return nil, err
				}
				names = append(names, name)
			}
			return map[string]any{"out": dir, "files": names}, nil
		},
		Text: func(w io.Writer, v any) {
			m := capreg.As[struct {
				Out   string
				Files []string
			}](v)
			fmt.Fprintf(w, "extracted %d verified attachments to %s\n", len(m.Files), m.Out)
		}})
}

// embedFile is mkv embed.
func embedFile(video, source, videoID string, hasOffset bool, offset int64, verified bool) (EmbedResult, error) {
	o := embed.Options{VideoID: videoID}
	if hasOffset {
		o.OffsetMs, o.SyncVerified = &offset, verified
	}
	comments.Version = version
	// Replaced comments are kept next to the video, in the folder's records.
	o.BackupDir = filepath.Join(filepath.Dir(video), library.VaultDir, "backup")
	r, err := embed.Embed(video, source, o)
	if errors.Is(err, comments.ErrNone) {
		return EmbedResult{Video: video, Reason: err.Error()}, nil
	}
	if err != nil {
		return EmbedResult{}, err
	}
	res := EmbedResult{Video: video, Embedded: true, Source: r.Source, Comments: r.Comments, Replaced: r.Replaced,
		Backup: r.Backup, Recovered: r.Recovered, OffsetMs: r.OffsetMs, SyncVerified: r.SyncVerified, Ms: r.Took.Milliseconds()}
	if r.Verification != nil {
		res.Packets = r.Verification.Packets
	}
	return res, nil
}

func printEmbed(w io.Writer, r EmbedResult) {
	if !r.Embedded {
		fmt.Fprintln(w, r.Reason)
		return
	}
	action := "embedded"
	if r.Replaced {
		action = "replaced embedded comments with"
	}
	sync := "unverified"
	if r.SyncVerified {
		sync = "verified"
	}
	fmt.Fprintf(w, "%s: %s %s (%d comments), offset %d ms %s; %d packets verified in %s\n",
		filepath.Base(r.Video), action, filepath.Base(r.Source), r.Comments, r.OffsetMs, sync,
		r.Packets, (time.Duration(r.Ms) * time.Millisecond).Round(time.Millisecond))
	if r.Backup != "" {
		fmt.Fprintln(w, "the comments it had are kept in", r.Backup)
	}
	if r.Recovered != "" {
		fmt.Fprintln(w, "an earlier interrupted embed was put right:", r.Recovered)
	}
}

// legacyNames are the file tools' spellings before they were capabilities.
var legacyNames = map[string]string{
	"import-xml": "snapshot import-xml", "import-zouryou-json": "snapshot import-zouryou",
	"validate": "snapshot validate", "derive": "snapshot derive", "pack": "snapshot pack",
	"attach": "mkv attach", "embed": "mkv embed", "verify": "mkv verify", "extract": "mkv extract",
}

// legacyCommand runs an old spelling (jusplay embed <video> …) as its
// capability, keeping what scripts rely on: -ffmpeg / -ffprobe are taken and
// ignored, and embed without a comment file exits 3 (as it always did).
// ok is false for anything else.
func legacyCommand(args []string, stdout, stderr io.Writer) (code int, ok bool) {
	if len(args) == 0 {
		return 0, false
	}
	name, is := legacyNames[args[0]]
	if !is {
		return 0, false
	}
	rest := []string{}
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-ffmpeg" || a == "-ffprobe" || a == "--ffmpeg" || a == "--ffprobe":
			i++ // and its value
		case strings.HasPrefix(a, "-ffmpeg=") || strings.HasPrefix(a, "-ffprobe=") || strings.HasPrefix(a, "--ffmpeg=") || strings.HasPrefix(a, "--ffprobe="):
		default:
			rest = append(rest, a)
		}
	}
	reg := newRegistry(nil)
	c, capArgs, found := reg.Resolve(append(strings.Fields(name), rest...))
	if !found {
		return 0, false
	}
	if c.ID != "mkv.embed" {
		return runCap(c, capArgs, os.Getenv("JUSPLAY_DATA"), stdout, stderr), true
	}
	inv, err := c.ParseCLI(capArgs, filepath.Abs)
	if err != nil {
		return report(c, err, inv.JSON, stdout, stderr), true
	}
	a := capreg.Args(inv.Params)
	r, err := embedFile(a.String("video"), a.String("comments"), a.String("video-id"), a.Has("offset-ms"), a.Int("offset-ms"), a.Bool("verified"))
	if err != nil {
		return report(c, err, inv.JSON, stdout, stderr), true
	}
	if !r.Embedded {
		fmt.Fprintln(stderr, r.Reason)
		return 3, true // "no comments": what the old embed said, and scripts read
	}
	c.PrintResult(stdout, r, inv.JSON)
	return 0, true
}
