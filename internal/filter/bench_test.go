package filter

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// A busy episode: 60 000 comments over 24 minutes (a heavily commented
// Niconico video has tens of thousands), with the cap and a few rules on.
func busy() []Thread {
	cmds := [][]string{{}, {"ue"}, {"shita", "big"}, {"red"}, {"184"}, {"small"}}
	var cs []Comment
	for i := 0; i < 60000; i++ {
		cs = append(cs, Comment{ID: fmt.Sprint("c", i), No: int64(i), VposMs: int64(i * 24), Body: fmt.Sprintf("コメント %d www", i%500),
			Commands: cmds[i%len(cmds)], UserID: fmt.Sprint("u", i%3000), Score: float64(-(i % 12000)), PostedAt: "2026-01-02T10:00:00+09:00"})
	}
	return []Thread{{ID: id("2"), Fork: "main", Comments: cs}}
}

func TestBusyEpisodeTiming(t *testing.T) {
	ts := busy()
	s := Defaults()
	s.NgWords = []string{`/^コメント 4\d\d/`, "草"}
	s.NgShare = "medium"
	for i := 0; i < 2; i++ { // warm-up
		Apply(ts, s, 1440)
	}
	const n = 10
	t0 := time.Now()
	var r Result
	for i := 0; i < n; i++ {
		r = Apply(ts, s, 1440)
	}
	apply := time.Since(t0) / n
	t1 := time.Now()
	b, _ := json.Marshal(r)
	enc := time.Since(t1)
	t.Logf("60000 comments: filter %s, answer %d KB encoded in %s (shown %d)", apply.Round(time.Microsecond), len(b)>>10, enc.Round(time.Microsecond), r.Stats.Shown)
}
