package player

// One live session (Jus contract 18). The window's pages report what they
// show (POST api/state), and whatever happens — a page's state, a change
// made through a capability, a command sent to the window — is an event
// on events.watch, which the pages themselves listen to for commands and
// the command line prints for `jusplay events watch`. A command
// (player.open, player.seek …) waits until the page says it is done and
// answers with the page's state then.
//
//	POST api/state   the page: {"state": {...}, "ack": "<command id>", "error": "..."}

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/Tieumi221E/Jus/capreg"
)

type live struct {
	mu      sync.Mutex
	state   map[string]any // as the page last reported it, plus "updated"
	last    string         // that state as JSON, without "updated"
	subs    map[*liveSub]bool
	waiters map[string]chan liveAck
}

type liveSub struct {
	ch   chan []byte
	page bool // a window page: it carries out commands
}

type liveAck struct {
	state  map[string]any
	result any
	err    string
}

// Event is one line of events.watch.
type Event struct {
	Kind   string         `json:"kind"` // hello, state, changed, command
	Time   time.Time      `json:"time"`
	Source capreg.Source  `json:"source"`
	Data   map[string]any `json:"data,omitempty"`
}

func (l *live) init() {
	if l.subs == nil {
		l.subs = map[*liveSub]bool{}
		l.waiters = map[string]chan liveAck{}
	}
}

// publish sends ev to every listener; a listener too slow to take it
// misses it rather than holding the window up.
func (s *Server) publish(kind string, src capreg.Source, data map[string]any) {
	b, err := json.Marshal(Event{Kind: kind, Time: time.Now(), Source: src, Data: data})
	if err != nil {
		return
	}
	s.live.mu.Lock()
	defer s.live.mu.Unlock()
	s.live.init()
	for sub := range s.live.subs {
		select {
		case sub.ch <- b:
		default:
		}
	}
}

func (s *Server) liveState() map[string]any {
	s.live.mu.Lock()
	defer s.live.mu.Unlock()
	out := map[string]any{}
	for k, v := range s.live.state {
		out[k] = v
	}
	return out
}

func (s *Server) pagesListening() int {
	s.live.mu.Lock()
	defer s.live.mu.Unlock()
	n := 0
	for sub := range s.live.subs {
		if sub.page {
			n++
		}
	}
	return n
}

// watch streams events until ctx ends; the first is "hello" with the
// current state. Pages (Via "window") are the ones commands go to.
func (s *Server) watch(ctx context.Context, emit func(any) error) error {
	sub := &liveSub{ch: make(chan []byte, 256), page: capreg.SourceOf(ctx).Via == "window"}
	s.live.mu.Lock()
	s.live.init()
	s.live.subs[sub] = true
	s.live.mu.Unlock()
	defer func() {
		s.live.mu.Lock()
		delete(s.live.subs, sub)
		s.live.mu.Unlock()
	}()
	if err := emit(Event{Kind: "hello", Time: time.Now(), Source: capreg.Source{Via: "window"}, Data: map[string]any{"state": s.liveState()}}); err != nil {
		return err
	}
	keep := time.NewTicker(20 * time.Second) // a line now and then: a dead reader is noticed
	defer keep.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case b := <-sub.ch:
			if err := emit(json.RawMessage(b)); err != nil {
				return err
			}
		case <-keep.C:
			if err := emit(Event{Kind: "tick", Time: time.Now(), Source: capreg.Source{Via: "window"}}); err != nil {
				return err
			}
		}
	}
}

// stateAPI is POST api/state, from the pages.
func (s *Server) stateAPI(w http.ResponseWriter, r *http.Request) {
	var req struct {
		State  map[string]any `json:"state"`
		Ack    string         `json:"ack"`
		Result any            `json:"result"`
		Error  string         `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.State != nil {
		// The same state again (a page reports on several events for one
		// change) is not a new event.
		same, _ := json.Marshal(req.State)
		req.State["updated"] = time.Now()
		s.live.mu.Lock()
		old := s.live.last
		s.live.state, s.live.last = req.State, string(same)
		s.live.mu.Unlock()
		if old != string(same) {
			s.publish("state", capreg.Source{Via: "window"}, req.State)
		}
	}
	if req.Ack != "" {
		s.live.mu.Lock()
		s.live.init()
		ch := s.live.waiters[req.Ack]
		delete(s.live.waiters, req.Ack)
		s.live.mu.Unlock()
		if ch != nil {
			ch <- liveAck{state: s.liveState(), result: req.Result, err: req.Error}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// Command sends cmd to the window's page and waits until it is carried
// out; the answer is what the page returned for it, else its state then.
func (s *Server) Command(ctx context.Context, cmd string, args map[string]any, wait time.Duration) (any, error) {
	if s.pagesListening() == 0 {
		return nil, errors.New("no window page is open to carry it out")
	}
	var b [8]byte
	rand.Read(b[:])
	id := hex.EncodeToString(b[:])
	ch := make(chan liveAck, 1)
	s.live.mu.Lock()
	s.live.init()
	s.live.waiters[id] = ch
	s.live.mu.Unlock()
	defer func() {
		s.live.mu.Lock()
		delete(s.live.waiters, id)
		s.live.mu.Unlock()
	}()
	data := map[string]any{"id": id, "cmd": cmd}
	for k, v := range args {
		data[k] = v
	}
	s.publish("command", capreg.SourceOf(ctx), data)
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case a := <-ch:
		if a.err != "" {
			return nil, errors.New(a.err)
		}
		if a.result != nil {
			return a.result, nil
		}
		return a.state, nil
	case <-t.C:
		return nil, errors.New("the window did not carry it out in time")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// registerLive adds the live session's capabilities.
func registerLive(r *capreg.Registry, with func(func(context.Context, *Server, capreg.Args) (any, error)) func(context.Context, capreg.Args) (any, error), get func() (*Server, error), entry capreg.Param) {
	r.Add(capreg.Cap{ID: "events.watch", Summary: "what happens in the window, as it happens (one JSON object per line): what it shows, changes and by whom, commands", Window: true,
		Stream: func(ctx context.Context, a capreg.Args, emit func(any) error) error {
			s, err := get()
			if err != nil {
				return err
			}
			return s.watch(ctx, emit)
		}})

	r.Add(capreg.Cap{ID: "player.status", Summary: "what the window shows now: the page, the video, its position, paused or playing", Window: true,
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			return map[string]any{"state": s.liveState(), "pages": s.pagesListening()}, nil
		})})

	cmd := func(name string, wait time.Duration, args func(ctx context.Context, s *Server, a capreg.Args) (map[string]any, error)) func(context.Context, capreg.Args) (any, error) {
		return with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			m := map[string]any{}
			if args != nil {
				var err error
				if m, err = args(ctx, s, a); err != nil {
					return nil, err
				}
			}
			return s.Command(ctx, name, m, wait)
		})
	}

	r.Add(capreg.Cap{ID: "player.open", Summary: "play a video in the window (from where it was left, or -at)", Window: true,
		Params: []capreg.Param{entry, {Name: "at", Kind: capreg.Number, Doc: "start here: seconds, or [h:]m:s"}},
		Run: cmd("open", 20*time.Second, func(ctx context.Context, s *Server, a capreg.Args) (map[string]any, error) {
			e, err := s.resolve(a.String("entry"))
			if err != nil {
				return nil, err
			}
			m := map[string]any{"entry": e.ID}
			if a.Has("at") {
				m["at"] = a.Float("at")
			} else if t, ok := linkMoment(a.String("entry")); ok {
				m["at"] = t // a link's moment
			}
			return m, nil
		})})

	r.Add(capreg.Cap{ID: "player.seek", Summary: "move the playing video to -to (a time) or by -by seconds", Window: true,
		Params: []capreg.Param{
			{Name: "to", Kind: capreg.Number, Doc: "seconds, or [h:]m:s"},
			{Name: "by", Kind: capreg.Number, Doc: "seconds forward (negative: back)"},
		},
		Run: cmd("seek", 10*time.Second, func(ctx context.Context, s *Server, a capreg.Args) (map[string]any, error) {
			if a.Has("to") == a.Has("by") {
				return nil, capreg.Usagef("player seek: give -to or -by")
			}
			if a.Has("to") {
				return map[string]any{"to": a.Float("to")}, nil
			}
			return map[string]any{"by": a.Float("by")}, nil
		})})

	r.Add(capreg.Cap{ID: "thumbs.make", Summary: "make the missing thumbnails (the window decodes the picture), or the one of a video", Window: true, Writes: true,
		Params: []capreg.Param{{Name: "entry", Kind: capreg.Ref, Positional: true, Doc: "only this video (default: every video without one)"}},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			var ids []string
			if a.Has("entry") {
				e, err := s.resolve(a.String("entry"))
				if err != nil {
					return nil, err
				}
				ids = []string{e.ID}
			} else {
				for _, e := range s.lib.Entries() {
					if !e.Missing && !s.thumbs.has(e) {
						ids = append(ids, e.ID)
					}
				}
			}
			if len(ids) == 0 {
				return map[string]any{"made": 0, "failed": []string{}}, nil
			}
			wait := min(time.Duration(len(ids))*15*time.Second+10*time.Second, 30*time.Minute)
			return s.Command(ctx, "thumbs", map[string]any{"ids": ids}, wait)
		})})

	r.Add(capreg.Cap{ID: "subs.generate", Summary: "make AI subtitles for a whole video (the window decodes the sound; needs the AI component); waits until done", Window: true, Writes: true,
		Params: []capreg.Param{entry},
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			e, err := s.resolve(a.String("entry"))
			if err != nil {
				return nil, err
			}
			// Before the window opens anything: nothing to undo when it cannot.
			if st := s.aiStatus(); st["available"] != true {
				return nil, fmt.Errorf("the AI component is not ready: %v (ai status; ai download -yes fetches the recommended one)", st["reason"])
			}
			return s.Command(ctx, "subs-all", map[string]any{"entry": e.ID}, 3*time.Hour)
		})})

	r.Add(capreg.Cap{ID: "player.fullscreen", Summary: "the playing video full screen (-on) or not (-on=false)", Window: true,
		Params: []capreg.Param{{Name: "on", Kind: capreg.Bool, Default: true, Doc: "false to leave full screen"}},
		Run: cmd("fullscreen", 5*time.Second, func(ctx context.Context, s *Server, a capreg.Args) (map[string]any, error) {
			return map[string]any{"on": a.Bool("on")}, nil
		})})

	r.Add(capreg.Cap{ID: "log.get", Summary: "the window's diagnostic log (kept in memory only; what \"copy diagnostic log\" copies)", Window: true,
		Run: with(func(ctx context.Context, s *Server, a capreg.Args) (any, error) {
			if s.LogText == nil {
				return "", nil
			}
			return s.LogText(), nil
		}),
		Text: func(w io.Writer, v any) { fmt.Fprint(w, v) }})

	r.Add(capreg.Cap{ID: "player.pause", Summary: "pause the playing video", Window: true, Run: cmd("pause", 5*time.Second, nil)})
	r.Add(capreg.Cap{ID: "player.play", Summary: "play (resume) the video", Window: true, Run: cmd("play", 5*time.Second, nil)})
	r.Add(capreg.Cap{ID: "player.back", Summary: "leave the video and go back to the library", Window: true, Run: cmd("back", 5*time.Second, nil)})
}
