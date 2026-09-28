// Command jusplay is the Jusplay player: the library and player windows,
// and every capability on the command line (caps.go, tools.go).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tieumi221E/Jus/procstat"
	"github.com/Tieumi221E/Jusplay/internal/derive"
	"github.com/Tieumi221E/Jusplay/internal/detzip"
	"github.com/Tieumi221E/Jusplay/internal/snapshot"
)

// version is set by build.ps1 from VERSION (-ldflags -X main.version=...).
var version = "0.0.0-dev"

var usage = `jusplay ` + version + `

  (no arguments) | library                   open the library window
  play       <file.mkv> [-comments file|snapshot] [-selftest [-stress 300s] | -serve]

The comment tools are commands like the others (snapshot import-xml,
snapshot derive, mkv embed, mkv verify …: jusplay help); their old
spellings (import-xml, embed, verify …) still work.
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
	// A link to a moment (jus://play/…, from a note): play it there, in the
	// window if one is open, else in a new one.
	if strings.HasPrefix(os.Args[1], "jus://") {
		c, _ := newRegistry(nil).Get("player.open")
		exit(runCap(c, os.Args[1:2], os.Getenv("JUSPLAY_DATA"), os.Stdout, os.Stderr))
	}
	// A video dropped on the exe, or opened with it: play it.
	if st, err := os.Stat(os.Args[1]); err == nil && !st.IsDir() && isVideo(os.Args[1]) {
		if handOver(os.Args[1]) {
			return
		}
		if err := cmdPlay(os.Args[1:2]); err != nil {
			fail(err)
		}
		return
	}
	if os.Args[1] == "-version" || os.Args[1] == "version" {
		fmt.Println("jusplay", version)
		exit(0)
	}
	switch os.Args[1] {
	case "help", "-help", "--help", "-h":
		exit(cmdHelp(os.Args[2:], os.Stdout))
	}
	// What the window can do, by name (caps.go).
	data, args := dataFlag(os.Args[1:])
	reg := newRegistry(nil)
	if c, rest, ok := reg.Resolve(args); ok {
		exit(runCap(c, rest, data, os.Stdout, os.Stderr))
	}
	// The file tools' spellings from before they were capabilities.
	if code, ok := legacyCommand(os.Args[1:], os.Stdout, os.Stderr); ok {
		exit(code)
	}
	cmds := map[string]func([]string) error{"play": cmdPlay, "library": cmdLibrary}
	f, ok := cmds[os.Args[1]]
	if !ok {
		fmt.Fprintf(os.Stderr, "jusplay: unknown command %q\n\n", os.Args[1])
		reg.Usage(os.Stderr, toolsUsage)
		exit(2)
	}
	if err := f(os.Args[2:]); err != nil {
		fail(err)
	}
}

// legacyToolFlags accepts -ffmpeg and -ffprobe, which these commands took
// until Jusplay read media itself (0.3.0), and ignores them: scripts
// written against the older commands keep working instead of failing on
// an unknown flag.
func legacyToolFlags(fs *flag.FlagSet) {
	fs.String("ffmpeg", "", "ignored (Jusplay reads media itself; kept for older scripts)")
	fs.String("ffprobe", "", "ignored (Jusplay reads media itself; kept for older scripts)")
}

// fail reports err where it can be seen (stderr, or a message box when
// started without a console) and exits.
// exit ends the process; with JUSPLAY_STATS set it first reports its peak
// memory on stderr (tools/bench.ps1: a short command's peak cannot be read
// from outside after it has gone).
func exit(code int) {
	procstat.Report("jusplay", "JUSPLAY_STATS")
	os.Exit(code)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	showError(err)
	exit(1)
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

// summarize is a derivation report, one line per thread and a total.
func summarize(w io.Writer, rep derive.Report) {
	for _, t := range rep.Threads {
		mark := " "
		if !t.Included {
			mark = "-"
		}
		fmt.Fprintf(w, "%s %s/%s: %d comments, %d dup, %d drift, %d conflict, %d invalid, %d low-confidence id, commentCount %v, no %d-%d with %d missing\n",
			mark, t.ThreadID, t.Fork, t.Comments, t.Duplicates, t.Drift, t.Conflicts, t.Invalid, t.LowConfidenceIdentity, t.CommentCountObserved,
			t.NumberRange[0], t.NumberRange[1], t.MissingNumbers)
	}
	fmt.Fprintf(w, "output %d comments; %d response errors, %d invalid, %d warnings, %d conflicts\n",
		rep.Output, len(rep.ResponseErrors), len(rep.Invalid), len(rep.Warnings), len(rep.Conflicts))
	if len(rep.Conflicts) > 0 {
		fmt.Fprintln(w, "conflicts keep the first occurrence; see the report for both locations")
	}
}
