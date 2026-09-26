package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/Tieumi221E/Jusplay/internal/ai"
	"github.com/Tieumi221E/Jusplay/internal/appdir"
	"github.com/Tieumi221E/Jusplay/internal/comments"
	"github.com/Tieumi221E/Jusplay/internal/library"
	"github.com/Tieumi221E/Jusplay/internal/memlog"
	"github.com/Tieumi221E/Jusplay/internal/player"
	"github.com/Tieumi221E/Jusplay/internal/shell"
	"github.com/Tieumi221E/Jusplay/internal/ui"
)

type appFlags struct {
	data, comments  string
	debugLog        bool
	selftest, serve bool
	stress          time.Duration
	bench           string
	benchSeconds    int
	layout          bool
	shots, shotsDir string
	trace           string
	subs            string
	dock            bool
	switchEps       bool
	scroll          bool
	thumbs          bool
}

func addAppFlags(fs *flag.FlagSet) *appFlags {
	a := &appFlags{}
	fs.StringVar(&a.data, "data", "", "the app's data folder (default: jusplay-data next to the executable, else %AppData%\\jusplay)")
	fs.BoolVar(&a.debugLog, "debug-log", false, "also write the log to <data>\\logs (it is otherwise kept in memory only)")
	fs.BoolVar(&a.serve, "serve", false, "no window: print the page URL and serve until interrupted (debugging)")
	return a
}

// cmdLibrary opens the library window (also what running with no
// arguments does).
func cmdLibrary(args []string) error {
	fs := flag.NewFlagSet("library", flag.ExitOnError)
	a := addAppFlags(fs)
	fs.BoolVar(&a.selftest, "selftest", false, "with -scroll: run the library's scrolling test, print its report and exit")
	fs.BoolVar(&a.scroll, "scroll", false, "with -selftest: scroll the home page down and up and time every frame")
	fs.BoolVar(&a.thumbs, "thumbs", false, "with -selftest: make every missing thumbnail in the page and report the times and sizes")
	if _, err := parse(fs, args, 0); err != nil {
		return err
	}
	return runApp(*a, "", "library.html")
}

func cmdPlay(args []string) error {
	fs := flag.NewFlagSet("play", flag.ExitOnError)
	a := addAppFlags(fs)
	fs.StringVar(&a.comments, "comments", "", "comment file (.json/.xml), snapshot directory or raw ZIP; remembered for this video")
	fs.BoolVar(&a.selftest, "selftest", false, "run the built-in playback test, print its report and exit")
	fs.DurationVar(&a.stress, "stress", 0, "with -selftest: run the seek/playback stress test for this long instead")
	fs.StringVar(&a.bench, "bench", "", "with -selftest: steady-state benchmark scenario (comments-display, comments-60, comments-video, no-comments, paused)")
	fs.IntVar(&a.benchSeconds, "bench-seconds", 25, "with -bench: measured seconds after a 5 s warm-up")
	fs.BoolVar(&a.layout, "layout", false, "with -selftest: only build the comment layout (several times) and report its fingerprint and timings")
	fs.StringVar(&a.shots, "shots", "", "with -selftest: comma-separated times (s); a 1920x1080 PNG of picture and comments at each goes to -shots-dir")
	fs.StringVar(&a.shotsDir, "shots-dir", "", "with -shots: where the PNGs go")
	fs.BoolVar(&a.switchEps, "switch", false, "with -selftest: switch to every other episode of the series on the page and time each to its first frame")
	fs.BoolVar(&a.dock, "dock", false, "with -selftest: open and close the episode layer while playing and time the frames of the move")
	fs.StringVar(&a.subs, "subs", "", "with -selftest: FROM,SECONDS: play from FROM with subtitles made while watching (needs the AI component) and report timings and lines")
	fs.StringVar(&a.trace, "trace", "", "with -selftest: FROM,SECONDS: seek to FROM, play, and report the playhead, buffered ranges and media events every 0.5 s")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	path, err := filepath.Abs(pos[0])
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return err
	}
	return runApp(*a, path, "index.html")
}

func runApp(a appFlags, video, page string) error {
	if !ui.Built() {
		return errors.New("page not built: run `npm --prefix web ci && npm --prefix web run build`, then rebuild")
	}
	// The log is kept in memory (the settings panel can copy it); a debug
	// run also writes it, and crashes, to a file.
	mem := memlog.New(4000)
	logger := log.New(mem, "", log.LstdFlags|log.Lmicroseconds)
	dir, portable := a.data, true
	if dir == "" {
		var err error
		if dir, portable, err = appdir.Dir(); err != nil {
			return err
		}
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if a.debugLog {
		if f, p := openLogFile(filepath.Join(dir, "logs")); f != nil {
			logger.SetOutput(io.MultiWriter(mem, f))
			fmt.Fprintf(os.Stderr, "log: %s\n", p)
		}
	}
	logger.Printf("data %s (portable %v)", dir, portable)
	temp, err := appdir.Temp(dir)
	if err != nil {
		return err
	}
	defer clearTemp(temp)
	comments.Version = version
	library.SetHide(appdir.Hide)
	folders := filepath.Join(dir, "folders.json")
	if a.data == "" {
		for _, m := range importLegacy(dir, folders) {
			logger.Print(m)
		}
	}
	lib, err := library.Open(folders)
	if err != nil {
		return fmt.Errorf("library: %w (the file is kept; move it away to start a new library)", err)
	}
	srv := player.New(lib, ui.FS(), player.OpenSettings(filepath.Join(dir, "settings.json")), filepath.Join(dir, "ui.json"), temp)
	srv.Log = logger
	srv.LogText = mem.String
	// AI subtitles: the component folder (recommended models and runtime,
	// downloaded on request) beside the executable, else in the data folder;
	// the backends' settings in ai.json. Nothing starts until subtitles are
	// first asked for.
	eng := ai.New(componentDir(dir), filepath.Join(dir, "ai.json"))
	eng.Log = logger.Printf
	logger.Printf("ai component folder %s", eng.Dir)
	srv.SetAI(eng)
	base, err := srv.Start()
	if err != nil {
		return err
	}
	defer srv.Close()

	target := base + page
	title := "Jusplay"
	if video != "" {
		e, err := lib.Ensure(video)
		if err != nil {
			return err
		}
		if a.comments != "" {
			// Remembered for later runs, which may start in another directory.
			abs, err := filepath.Abs(a.comments)
			if err != nil {
				return err
			}
			if err := lib.Update(e.ID, func(x *library.Entry) { x.Comments = abs }); err != nil {
				return err
			}
		}
		srv.PreOpen(e.ID) // alongside the window's creation
		q := url.Values{"id": {e.ID}}
		if a.selftest {
			q.Set("selftest", "1")
			if a.stress > 0 {
				q.Set("stress", fmt.Sprint(int(a.stress.Seconds())))
			}
			if a.layout {
				q.Set("layout", "1")
			}
			if a.shots != "" {
				q.Set("shots", a.shots)
				srv.ShotsDir = a.shotsDir
			}
			if a.subs != "" {
				q.Set("subs", a.subs)
			}
			if a.trace != "" {
				q.Set("trace", a.trace)
			}
			if a.dock {
				q.Set("dock", "1")
			}
			if a.switchEps {
				q.Set("switch", "1")
			}
			if a.bench != "" {
				q.Set("bench", a.bench)
				q.Set("seconds", fmt.Sprint(a.benchSeconds))
			}
		}
		target += "?" + q.Encode()
		title = filepath.Base(video) + " — Jusplay"
		logger.Printf("play %s (entry %s)", video, e.ID)
	} else {
		srv.ScanAsync() // pick up changes since the last run
		logger.Printf("library")
		if a.selftest && a.scroll {
			target += "?selftest=1&scroll=1"
		} else if a.selftest && a.thumbs {
			target += "?selftest=1&thumbs=1"
		}
	}
	if a.serve {
		fmt.Println(target)
		select {}
	}
	// One WebView2 profile per process: a window closed a moment ago may
	// still hold its own for a while, and a new one must not wait for it
	// (reusing it made the engine fail to start). Old ones go with the
	// temporary folder at the next start.
	win, err := shell.Open(title, target, srv.Dark(), filepath.Join(temp, fmt.Sprintf("webview2-%d", os.Getpid())))
	if err != nil {
		return err
	}
	if a.selftest {
		report := make(chan []byte, 1)
		srv.OnSelftest = func(b []byte) { report <- b }
		go func() {
			select {
			case b := <-report:
				os.Stdout.Write(append(b, '\n'))
				// For the startup timeline: the page's times count from its
				// navigation start (timeOrigin in the report).
				fmt.Fprintf(os.Stderr, "process start %d\n", processStart.UnixMilli())
			case <-time.After(a.stress + 5*time.Minute):
				fmt.Fprintln(os.Stderr, "selftest: no report in time")
			}
			win.Close()
		}()
	}
	win.Run()
	logger.Printf("window closed")
	return nil
}

// openLogFile starts a log file in dir (a debug run) and routes fatal
// runtime errors (crashes) there too, so a crash that closes the window
// leaves a stack trace. Old logs beyond the newest 20 are removed.
func openLogFile(dir string) (*os.File, string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, ""
	}
	if old, _ := filepath.Glob(filepath.Join(dir, "jusplay-*.log")); len(old) > 20 {
		for _, f := range old[:len(old)-20] {
			os.Remove(f)
		}
	}
	p := filepath.Join(dir, "jusplay-"+time.Now().Format("20060102-150405")+".log")
	f, err := os.Create(p)
	if err != nil {
		return nil, ""
	}
	debug.SetCrashOutput(f, debug.CrashOptions{})
	return f, p
}

// clearTemp empties the temporary folder once the window has gone. The
// engine's processes may hold files for a moment after that; what they
// still hold is removed at the next start (appdir.Temp).
func clearTemp(t string) {
	for i := 0; i < 10; i++ {
		appdir.ClearTemp(t)
		if _, err := os.Stat(t); err != nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// componentDir is where the AI component lives: "components\ai" beside the
// executable when it is there or can be made there (a portable copy keeps
// it), else in the data folder.
func componentDir(data string) string {
	if exe, err := os.Executable(); err == nil {
		d := filepath.Join(filepath.Dir(exe), "components", "ai")
		if _, err := os.Stat(d); err == nil {
			return d
		}
		if f, err := os.CreateTemp(filepath.Dir(exe), ".probe-*"); err == nil {
			f.Close()
			os.Remove(f.Name())
			return d
		}
	}
	return filepath.Join(data, "components", "ai")
}
