package main

// The command line over the capability registry (internal/capreg): all
// the window can do, by name, and `help -json` describing it. When a
// Jusplay window is open the call runs in it — it shows the effect at once,
// and there is one writer — found through instance.json in the data
// folder; otherwise it runs here, on the same files, the same way.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Tieumi221E/Jus/capreg"
	"github.com/Tieumi221E/Jus/instance"
	"github.com/Tieumi221E/Jusplay/internal/appdir"
	"github.com/Tieumi221E/Jusplay/internal/player"
	"github.com/Tieumi221E/Jusplay/internal/shell"
)

func newRegistry(get func() (*player.Server, error)) *capreg.Registry {
	r := capreg.New("jusplay", version)
	player.Register(r, get)
	registerManifest(r)
	registerTools(r)
	return r
}

// announce and findInstance: the window of a data folder, announced there
// and found by the command line (the Jus module's instance package).
func announce(dir, url string) func() { return instance.Announce(dir, "jusplay", version, url) }

func findInstance(dir string) (capreg.Remote, bool) {
	rm, _, ok := instance.Find(dir)
	return rm, ok
}

// handOver gives a video opened from Explorer (or `jusplay <file>`) to
// the window already open, if there is one: one window, one writer.
func handOver(video string) bool {
	dir, err := dataDir(os.Getenv("JUSPLAY_DATA"))
	if err != nil {
		return false
	}
	rm, in, ok := instance.Find(dir)
	if !ok {
		return false
	}
	rm.Source = capreg.Source{Via: "cli"}
	shell.AllowForeground(in.PID)
	abs, err := filepath.Abs(video)
	if err != nil {
		return false
	}
	_, err = rm.Call(context.Background(), "player.open", map[string]any{"entry": abs})
	return err == nil
}

// dataFlag takes -data <dir> (or -data=<dir>) out of args: the data folder
// for the command line, as for the window. JUSPLAY_DATA is the default.
func dataFlag(args []string) (string, []string) {
	dir := os.Getenv("JUSPLAY_DATA")
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-data" || a == "--data":
			if i+1 < len(args) {
				dir = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "-data=") || strings.HasPrefix(a, "--data="):
			_, dir, _ = strings.Cut(a, "=")
		default:
			rest = append(rest, a)
		}
	}
	return dir, rest
}

// dataDir is the folder the command line works on: -data, or the default.
func dataDir(flag string) (string, error) {
	if flag != "" {
		return filepath.Abs(flag)
	}
	d, _, err := appdir.Dir()
	return d, err
}

// runCap runs one capability from the command line and returns the exit code.
func runCap(c *capreg.Cap, args []string, data string, stdout, stderr io.Writer) int {
	inv, err := c.ParseCLI(args, filepath.Abs)
	if err != nil {
		return report(c, err, inv.JSON, stdout, stderr)
	}
	dir, err := dataDir(data)
	if err != nil {
		return report(c, err, inv.JSON, stdout, stderr)
	}
	rm, ok := findInstance(dir)
	if !ok && c.Window {
		rm, err = startWindow(dir, data)
		ok = err == nil
		if err != nil {
			return report(c, err, inv.JSON, stdout, stderr)
		}
	}
	rm.Source = inv.Source
	ctx := context.Background()
	if c.Streams {
		if !ok {
			return report(c, errors.New("this needs the Jusplay window"), inv.JSON, stdout, stderr)
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		err = rm.Stream(ctx, c.ID, inv.Params, func(v any) error { return enc.Encode(v) })
		if err != nil {
			return report(c, err, inv.JSON, stdout, stderr)
		}
		return capreg.ExitOK
	}
	var v any
	if ok {
		v, err = rm.Call(ctx, c.ID, inv.Params)
	} else {
		v, err = callHere(capreg.WithSource(ctx, inv.Source), dir, c, inv.Params)
	}
	if err != nil {
		return report(c, err, inv.JSON, stdout, stderr)
	}
	c.PrintResult(stdout, v, inv.JSON)
	return capreg.ExitOK
}

// startWindow opens the library window on dir (for a command that needs
// the window) and waits until it answers. JUSPLAY_NO_WINDOW=1 forbids it
// (tests, headless machines).
func startWindow(dir, data string) (capreg.Remote, error) {
	if os.Getenv("JUSPLAY_NO_WINDOW") == "1" {
		return capreg.Remote{}, errors.New("this needs the Jusplay window, and none is open")
	}
	exe, err := os.Executable()
	if err != nil {
		return capreg.Remote{}, err
	}
	args := []string{"library"}
	if data != "" {
		args = append(args, "-data", dir)
	}
	cmd := exec.Command(exe, args...)
	if err := cmd.Start(); err != nil {
		return capreg.Remote{}, fmt.Errorf("starting the window: %w", err)
	}
	go cmd.Wait()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if rm, ok := findInstance(dir); ok {
			return rm, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return capreg.Remote{}, errors.New("the Jusplay window did not start in time")
}

// callHere runs the call in this process, on the data folder dir.
func callHere(ctx context.Context, dir string, c *capreg.Cap, params map[string]any) (any, error) {
	var once sync.Once
	var env *appEnv
	var openErr error
	get := func() (*player.Server, error) {
		once.Do(func() { env, openErr = openApp(dir, false, false) })
		if openErr != nil {
			return nil, openErr
		}
		return env.srv, nil
	}
	reg := newRegistry(get)
	defer func() {
		if env != nil {
			env.srv.Close()
		}
	}()
	b, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	v, err := reg.Call(ctx, c.ID, b)
	return capreg.Generic(v), err
}

// report prints err (as {"error","kind"} with -json) and returns its exit code.
func report(c *capreg.Cap, err error, asJSON bool, stdout, stderr io.Writer) int {
	e := capreg.AsError(err)
	if asJSON {
		json.NewEncoder(stderr).Encode(capreg.ErrorOut(err))
	} else {
		fmt.Fprintf(stderr, "jusplay: %s\n", e.Msg)
		if e.Plan != nil {
			b, _ := json.MarshalIndent(e.Plan, "", "  ")
			fmt.Fprintf(stderr, "it would do:\n%s\n", b)
		}
	}
	return capreg.ExitCode(err)
}

// The commands that are not the window's (tools for comment snapshots and
// MKV attachments), listed after the registry's.
const toolsUsage = `Tools for comment snapshots and MKV attachments:
  import-xml <file.xml> -video-id ID -o <snapshot-dir>
  import-zouryou-json <file.json> -video-id ID -o <snapshot-dir>
  validate   <snapshot-dir|raw.zip>
  derive     <snapshot-dir|raw.zip> -o playback.json [-report r.json] [-forks owner,main,easy]
  pack       <snapshot-dir> -o raw.zip
  attach     <in.mkv> <snapshot-dir> -o <out.mkv> [-forks ...] [-offset-ms N]
  embed      <video.mkv> [comments.json|snapshot-dir|raw.zip|.xml]   in place; default: same-name .json
  verify     <file.mkv>
  extract    <file.mkv> -o <dir>

The window:
  (no arguments) | library      open the library
  <video file> | play <file>    play a video

`

func cmdHelp(args []string, stdout io.Writer) int {
	reg := newRegistry(func() (*player.Server, error) { return nil, errors.New("help only") })
	for _, a := range args {
		if a == "-json" || a == "--json" {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			enc.Encode(reg.Help(
				"-data <folder> (or JUSPLAY_DATA) selects the app's data folder; the default is jusplay-data beside the executable, else %AppData%\\jusplay.",
				"A video is named by its path or by the id library list gives.",
				"Writes that take -base fail with exit 3 when the video's records changed since that version; without -base they apply to the current state and never undo someone else's change.",
				"Other commands (comment snapshots, MKV attachments, the window) are listed by `jusplay help`.",
			))
			return capreg.ExitOK
		}
	}
	reg.Usage(stdout, toolsUsage)
	return capreg.ExitOK
}
