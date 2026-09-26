package library

import (
	"fmt"
	"sort"
	"strings"
)

// Versions groups the files that are the same episode (same series in the
// same added folder, same season and episode number; for example a
// release and its re-encode in "anime_processed") and picks the one that
// stands for it. It returns, for each file that is not that one, the one
// that is (alt -> primary), and for each primary its other versions.
// Files without an episode number are never grouped.
//
// The primary is, in order: one that can be played (probed, not missing);
// one with comments (attached, or a linked source); one watched or in
// progress; the newest; then by path, so the choice is stable.
func Versions(entries []Entry) (primaryOf map[string]string, others map[string][]string) {
	groups := map[string][]Entry{}
	for _, e := range entries {
		if e.Episode == nil {
			continue
		}
		k := fmt.Sprintf("%s\x00%s\x00%d\x00%g", e.Folder, e.Series, seasonOf(e), *e.Episode)
		groups[k] = append(groups[k], e)
	}
	primaryOf, others = map[string]string{}, map[string][]string{}
	for _, g := range groups {
		if len(g) < 2 {
			continue
		}
		sort.Slice(g, func(i, j int) bool { return better(g[i], g[j]) })
		for _, e := range g[1:] {
			primaryOf[e.ID] = g[0].ID
			others[g[0].ID] = append(others[g[0].ID], e.ID)
		}
	}
	return primaryOf, others
}

func better(a, b Entry) bool {
	playable := func(e Entry) bool { return !e.Missing && e.Probe != nil && e.Probe.Error == "" }
	comments := func(e Entry) bool { return e.Comments != "" || (e.Probe != nil && e.Probe.Attached) }
	started := func(e Entry) bool { return e.Watched || e.Position > 0 }
	for _, f := range []func(Entry) bool{playable, comments, started} {
		if f(a) != f(b) {
			return f(a)
		}
	}
	if !a.ModTime.Equal(b.ModTime) {
		return a.ModTime.After(b.ModTime)
	}
	return strings.ToLower(a.Path) < strings.ToLower(b.Path)
}
