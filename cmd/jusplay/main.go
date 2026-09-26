// Command jusplay archives Niconico comment snapshots and attaches them
// to local MKV files.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tieumi221E/Jusplay/internal/bundle"
	"github.com/Tieumi221E/Jusplay/internal/derive"
	"github.com/Tieumi221E/Jusplay/internal/detzip"
	"github.com/Tieumi221E/Jusplay/internal/importxml"
	"github.com/Tieumi221E/Jusplay/internal/importzouryou"
	"github.com/Tieumi221E/Jusplay/internal/mkv"
	"github.com/Tieumi221E/Jusplay/internal/snapshot"
)

// version is set by build.ps1 from VERSION (-ldflags -X main.version=...).
var version = "0.0.0-dev"

var usage = `jusplay ` + version + `

  import-xml <file.xml> -video-id ID -o <snapshot-dir>
  import-zouryou-json <file.json> -video-id ID -o <snapshot-dir>
  validate   <snapshot-dir|raw.zip>
  derive     <snapshot-dir|raw.zip> -o playback.json [-report r.json] [-forks owner,main,easy]
  pack       <snapshot-dir> -o raw.zip
  attach     <in.mkv> <snapshot-dir> -o <out.mkv> [-forks ...] [-offset-ms N]
  embed      <video.mkv> [comments.json|snapshot-dir|raw.zip|.xml]   in place; default: same-name .json
  (no arguments) | library                   open the library window
  play       <file.mkv> [-comments file|snapshot] [-selftest [-stress 300s] | -serve]
  verify     <file.mkv>
  extract    <file.mkv> -o <dir>

`

// processStart is when the process began, for startup measurements.
var processStart = time.Now()

func main() {
	if len(os.Args) < 2 {
		// Double-clicked: open the library.
		if err := cmdLibrary(nil); err != nil {
			fail(err)
		}
		return
	}
	// A video dropped on the exe, or opened with it: play it.
	if st, err := os.Stat(os.Args[1]); err == nil && !st.IsDir() && isVideo(os.Args[1]) {
		if err := cmdPlay(os.Args[1:2]); err != nil {
			fail(err)
		}
		return
	}
	if os.Args[1] == "-version" || os.Args[1] == "version" {
		fmt.Println("jusplay", version)
		return
	}
	cmds := map[string]func([]string) error{
		"import-xml": cmdImportXML, "import-zouryou-json": cmdImportZouryou, "validate": cmdValidate, "derive": cmdDerive,
		"pack": cmdPack, "attach": cmdAttach, "verify": cmdVerify, "extract": cmdExtract,
		"play": cmdPlay, "embed": cmdEmbed, "library": cmdLibrary,
	}
	f, ok := cmds[os.Args[1]]
	if !ok {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err := f(os.Args[2:]); err != nil {
		fail(err)
	}
}

// fail reports err where it can be seen (stderr, or a message box when
// started without a console) and exits.
func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	showError(err)
	os.Exit(1)
}

func isVideo(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".mkv", ".mp4", ".m4v", ".webm":
		return true
	}
	return false
}

// parse accepts flags before and after positional arguments.
func parse(fs *flag.FlagSet, args []string, npos int) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
	if len(pos) != npos {
		return nil, fmt.Errorf("want %d argument(s), got %d\n\n%s", npos, len(pos), usage)
	}
	return pos, nil
}

func forks(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}


// load opens a snapshot directory or a raw ZIP.
func load(path string) (*snapshot.Snapshot, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return snapshot.Load(os.DirFS(path))
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	zfs, err := detzip.Open(b)
	if err != nil {
		return nil, err
	}
	return snapshot.Load(zfs)
}

// writeNew refuses to overwrite and writes via a temporary file.
func writeNew(path string, b []byte) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}
	tmp := path + ".partial"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func printJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func cmdImportXML(args []string) error {
	fs := flag.NewFlagSet("import-xml", flag.ExitOnError)
	videoID := fs.String("video-id", "", "Niconico video id, e.g. so00000001")
	out := fs.String("o", "", "new snapshot directory")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	if *videoID == "" || *out == "" {
		return errors.New("-video-id and -o are required")
	}
	b, err := os.ReadFile(pos[0])
	if err != nil {
		return err
	}
	c, err := importxml.Import(b, *videoID, filepath.Base(pos[0]), version, *out)
	if err != nil {
		return err
	}
	fmt.Printf("snapshot %s written to %s (status partial: legacy XML)\n", c.SnapshotID, *out)
	return nil
}

func cmdImportZouryou(args []string) error {
	fs := flag.NewFlagSet("import-zouryou-json", flag.ExitOnError)
	videoID := fs.String("video-id", "", "Niconico video id, e.g. so00000001")
	out := fs.String("o", "", "new snapshot directory")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	if *videoID == "" || *out == "" {
		return errors.New("-video-id and -o are required")
	}
	b, err := os.ReadFile(pos[0])
	if err != nil {
		return err
	}
	c, err := importzouryou.Import(b, *videoID, filepath.Base(pos[0]), version, *out)
	if err != nil {
		return err
	}
	fmt.Printf("snapshot %s written to %s (status partial: コメント増量 export)\n", c.SnapshotID, *out)
	return nil
}

func cmdValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	s, err := load(pos[0])
	if err != nil {
		return err
	}
	fmt.Printf("ok: %s %s, %d response(s), origin %s, status %s\n",
		s.Capture.VideoID, s.Capture.SnapshotID, len(s.Capture.Responses), s.Capture.Origin, s.Report.Status)
	return nil
}

func cmdDerive(args []string) error {
	fs := flag.NewFlagSet("derive", flag.ExitOnError)
	out := fs.String("o", "", "playback JSON output")
	reportPath := fs.String("report", "", "derivation report output")
	forkList := fs.String("forks", "", "comma-separated forks (default all)")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	if *out == "" {
		return errors.New("-o is required")
	}
	s, err := load(pos[0])
	if err != nil {
		return err
	}
	playback, rep, err := derive.Derive(s, derive.Options{Forks: forks(*forkList)})
	if err != nil {
		return err
	}
	if err := writeNew(*out, playback); err != nil {
		return err
	}
	if *reportPath != "" {
		rb, err := snapshot.MarshalDoc(rep)
		if err != nil {
			return err
		}
		if err := writeNew(*reportPath, rb); err != nil {
			return err
		}
	}
	summarize(rep)
	return nil
}

func summarize(rep *derive.Report) {
	for _, t := range rep.Threads {
		mark := " "
		if !t.Included {
			mark = "-"
		}
		fmt.Printf("%s %s/%s: %d comments, %d dup, %d drift, %d conflict, %d invalid, %d low-confidence id, commentCount %v, no %d-%d with %d missing\n",
			mark, t.ThreadID, t.Fork, t.Comments, t.Duplicates, t.Drift, t.Conflicts, t.Invalid, t.LowConfidenceIdentity, t.CommentCountObserved,
			t.NumberRange[0], t.NumberRange[1], t.MissingNumbers)
	}
	fmt.Printf("output %d comments; %d response errors, %d invalid, %d warnings, %d conflicts\n",
		rep.Output, len(rep.ResponseErrors), len(rep.Invalid), len(rep.Warnings), len(rep.Conflicts))
	if len(rep.Conflicts) > 0 {
		fmt.Println("conflicts keep the first occurrence; see the report for both locations")
	}
}

func cmdPack(args []string) error {
	fs := flag.NewFlagSet("pack", flag.ExitOnError)
	out := fs.String("o", "", "ZIP output")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	if *out == "" {
		return errors.New("-o is required")
	}
	s, err := load(pos[0])
	if err != nil {
		return err
	}
	z, err := detzip.Write(s.Files())
	if err != nil {
		return err
	}
	if err := writeNew(*out, z); err != nil {
		return err
	}
	fmt.Printf("%s: %d bytes, sha256 %s\n", *out, len(z), snapshot.SHA256Hex(z))
	return nil
}

func cmdAttach(args []string) error {
	fs := flag.NewFlagSet("attach", flag.ExitOnError)
	out := fs.String("o", "", "new MKV output (never overwritten)")
	forkList := fs.String("forks", "", "comma-separated forks (default all)")
	offset := fs.Int64("offset-ms", 0, "nicoMs = localMs + offset")
	rmode := fs.String("renderer-mode", "default", "niconicomments mode")
	pos, err := parse(fs, args, 2)
	if err != nil {
		return err
	}
	if *out == "" {
		return errors.New("-o is required")
	}
	s, err := load(pos[1])
	if err != nil {
		return err
	}
	probe, err := mkv.ProbeFile(pos[0])
	if err != nil {
		return err
	}
	atts, rep, err := bundle.Build(s, probe, bundle.Options{
		Derive:    derive.Options{Forks: forks(*forkList)},
		OffsetMs:  *offset,
		Renderer:  bundle.Renderer{Name: "niconicomments", Version: "0.4.1-jusplay", Mode: *rmode, Fonts: "Jus Sans 2.005"},
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	summarize(rep)
	start := time.Now()
	v, err := mkv.Attach(pos[0], *out, atts)
	if err != nil {
		return err
	}
	fmt.Printf("wrote %s in %s\n", *out, time.Since(start).Round(time.Millisecond))
	printJSON(v)
	return nil
}

func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	files, err := mkv.Extract(pos[0])
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("no jusplay attachments")
	}
	m, err := bundle.Check(files)
	if err != nil {
		return err
	}
	fmt.Printf("ok: %s snapshot %s, status %s, %d attachments consistent, playback re-derives identically\n",
		m.VideoID, m.SnapshotID, m.CaptureStatus, len(files))
	return nil
}

func cmdExtract(args []string) error {
	fs := flag.NewFlagSet("extract", flag.ExitOnError)
	out := fs.String("o", "", "output directory (must not exist)")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	if *out == "" {
		return errors.New("-o is required")
	}
	if _, err := os.Stat(*out); err == nil {
		return fmt.Errorf("%s already exists", *out)
	}
	files, err := mkv.Extract(pos[0])
	if err != nil {
		return err
	}
	if _, err := bundle.Check(files); err != nil {
		return fmt.Errorf("attachments failed verification, not extracted: %w", err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(*out, name), b, 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("extracted %d verified attachments to %s\n", len(files), *out)
	return nil
}
