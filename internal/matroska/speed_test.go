package matroska

import (
	"os"
	"testing"
	"time"
)

// TestIndexSpeed times Open and Frames on JUSPLAY_SPEED (a development measurement).
func TestIndexSpeed(t *testing.T) {
	path := os.Getenv("JUSPLAY_SPEED")
	if path == "" {
		t.Skip("JUSPLAY_SPEED not set")
	}
	for i := 0; i < 3; i++ {
		t0 := time.Now()
		f, c, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		t1 := time.Now()
		fr, err := f.Frames(nil)
		if err != nil {
			t.Fatal(err)
		}
		c.Close()
		t.Logf("open %v, frames %v (%d frames)", t1.Sub(t0), time.Since(t1), len(fr))
	}
}
