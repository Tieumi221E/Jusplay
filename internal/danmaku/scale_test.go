package danmaku

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestScale measures loading and analysing a large corpus made from real
// comments (JUSPLAY_DANMAKU_FILES) scaled to JUSPLAY_DANMAKU_SCALE comments.
// The copies keep the real timing (±3 s jitter); 60 % repeat a real line,
// 30 % add a common ending (new writings of the same phrase), 10 % join two
// lines (new phrases), so distinct phrases grow with size as they would.
func TestScale(t *testing.T) {
	list, n := os.Getenv("JUSPLAY_DANMAKU_FILES"), os.Getenv("JUSPLAY_DANMAKU_SCALE")
	if list == "" || n == "" {
		t.Skip("JUSPLAY_DANMAKU_FILES and JUSPLAY_DANMAKU_SCALE not set")
	}
	target, _ := strconv.Atoi(n)
	var seed []Comment
	for _, p := range strings.Split(list, ";") {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var threads []struct{ Comments []Comment }
		if err := json.Unmarshal(b, &threads); err != nil {
			t.Fatal(err)
		}
		for _, th := range threads {
			seed = append(seed, th.Comments...)
		}
	}
	r := rand.New(rand.NewSource(1))
	endings := []string{"w", "ww", "草", "!", "!!", "ね", "よ", "かわいい", "すき", "?", "…", "〜"}
	var buf bytes.Buffer
	buf.WriteString(`[{"fork":"main","comments":[`)
	for i := 0; i < target; i++ {
		c := seed[r.Intn(len(seed))]
		switch x := r.Float64(); {
		case x < 0.3:
			c.Body += endings[r.Intn(len(endings))]
		case x < 0.4:
			c.Body += seed[r.Intn(len(seed))].Body
		}
		c.VposMs = max(0, c.VposMs+int64(r.Intn(6000)-3000))
		c.UserID = fmt.Sprintf("%s-%d", c.UserID, i%97)
		if i > 0 {
			buf.WriteByte(',')
		}
		b, _ := json.Marshal(c)
		buf.Write(b)
	}
	buf.WriteString(`]}]`)
	raw := buf.Len()
	peak := uint64(0)
	stop := make(chan struct{})
	go func() {
		var ms runtime.MemStats
		for {
			select {
			case <-stop:
				return
			case <-time.After(20 * time.Millisecond):
				runtime.ReadMemStats(&ms)
				peak = max(peak, ms.HeapInuse)
			}
		}
	}()
	runtime.GC()
	var base runtime.MemStats
	runtime.ReadMemStats(&base)
	t0 := time.Now()
	c, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	load := time.Since(t0)
	t1 := time.Now()
	rep := Analyze(c, 0)
	analyse := time.Since(t1)
	close(stop)
	t.Logf("%d comments (%d MB of JSON), %d distinct phrases, %d accounts", c.Len(), raw>>20, len(c.Keys), len(c.Users))
	t.Logf("load %s (%.0f comments/s), analyse %s; parts %v ms", load.Round(time.Millisecond), float64(c.Len())/load.Seconds(), analyse.Round(time.Millisecond), rep.Timing)
	t.Logf("peak heap in use %d MB above the %d MB before loading (the JSON itself included); %d CPUs, %s", (peak-base.HeapInuse)>>20, base.HeapInuse>>20, runtime.NumCPU(), runtime.GOARCH)
	t.Logf("hotspots %d, terms %d (first %v), families %d", len(rep.Hotspots), len(rep.Terms), rep.Terms[:min(5, len(rep.Terms))], len(rep.Families))
}
