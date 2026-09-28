package player

import (
	"sort"

	"github.com/Tieumi221E/Jus/capreg"
	"github.com/Tieumi221E/Jusplay/internal/filter"
)

// The comments the player shows: the session's comment data through the
// filters (internal/filter). The page asks for the positions kept and
// draws those; the command line lists the same comments.

// threadsOf is sess's comment data as filtering reads it, parsed once.
func (sess *Session) threadsOf() ([]filter.Thread, error) {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if sess.threads == nil && sess.threadsErr == nil {
		sess.threads, sess.threadsErr = filter.Load(sess.Comments.playback)
	}
	return sess.threads, sess.threadsErr
}

// FilterComments is which of sess's comments the settings document doc
// shows.
func (s *Server) FilterComments(sess *Session, doc []byte) (filter.Result, error) {
	if sess.Comments.playback == nil {
		return filter.Result{}, capreg.NotFoundf("no comments for this video")
	}
	ts, err := sess.threadsOf()
	if err != nil {
		return filter.Result{}, err
	}
	return filter.Apply(ts, filter.FromDocument(doc), sess.Media.Duration), nil
}

// shownComment is one line of comments list.
type shownComment struct {
	Time     float64  `json:"time"` // seconds into the video, before the comment offset
	Fork     string   `json:"fork"`
	No       int64    `json:"no"`
	Body     string   `json:"body"`
	Commands []string `json:"commands,omitempty"`
}

// ListComments is sess's comments in [from, to) (to 0: to the end), as
// shown under the saved settings, or all of them.
func (s *Server) ListComments(sess *Session, all bool, from, to float64, limit int) (map[string]any, error) {
	if sess.Comments.playback == nil {
		return nil, capreg.NotFoundf("no comments for this video")
	}
	ts, err := sess.threadsOf()
	if err != nil {
		return nil, err
	}
	var res filter.Result
	if all {
		res.Keep = make([][]int, len(ts))
		for i, t := range ts {
			for j := range t.Comments {
				res.Keep[i] = append(res.Keep[i], j)
			}
			res.Stats.Total += len(t.Comments)
		}
		res.Stats.Shown = res.Stats.Total
	} else {
		res = filter.Apply(ts, filter.FromDocument(s.settings.Get()), sess.Media.Duration)
	}
	out := []shownComment{}
	for i, t := range ts {
		for _, j := range res.Keep[i] {
			c := t.Comments[j]
			tm := float64(c.VposMs) / 1000
			if tm < from || (to > 0 && tm >= to) {
				continue
			}
			out = append(out, shownComment{Time: tm, Fork: t.Fork, No: c.No, Body: c.Body, Commands: c.Commands})
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Time < out[b].Time })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return map[string]any{"comments": out, "shown": res.Stats.Shown, "total": res.Stats.Total, "filtered": !all, "stats": res.Stats}, nil
}
