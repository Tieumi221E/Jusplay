package filter

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// testdata/filter/equivalence.json holds random synthetic comment data and
// settings with what the page's own filter (web/src/filter.ts as of commit
// 9f8bccf, before it moved here) kept, made by a one-off script over it
// (seeded pseudo-random threads of up to 120 comments per fork, mixed
// commands, users, scores, dates and multi-line bodies; random filters).
// The Go filter must keep exactly the same comments and count the same.
func TestSameAsThePage(t *testing.T) {
	b, err := os.ReadFile("../../testdata/filter/equivalence.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Threads  []Thread        `json:"threads"`
		Settings json.RawMessage `json:"settings"`
		Duration float64         `json:"duration"`
		Keep     [][]int         `json:"keep"`
		Stats    Stats           `json:"stats"`
	}
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	comments := 0
	for i, c := range cases {
		r := Apply(c.Threads, FromDocument(c.Settings), c.Duration)
		for _, th := range c.Threads {
			comments += len(th.Comments)
		}
		if fmt.Sprint(r.Keep) != fmt.Sprint(c.Keep) {
			t.Errorf("case %d: kept differs\n go   %v\n page %v", i, r.Keep, c.Keep)
			continue
		}
		if fmt.Sprint(r.Stats.ByRule) != fmt.Sprint(c.Stats.ByRule) || r.Stats.Shown != c.Stats.Shown || r.Stats.Cap != c.Stats.Cap {
			t.Errorf("case %d: stats\n go   %+v\n page %+v", i, r.Stats, c.Stats)
		}
	}
	t.Logf("%d cases, %d comments: the same as the page", len(cases), comments)
}
