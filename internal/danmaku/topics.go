package danmaku

import (
	"math"
	"sort"
	"strings"
	"sync"
)

// HistBins is how many stretches a video is cut into for the time
// distribution of a phrase, a term or a topic.
const HistBins = 48

// Topic is a group of terms talked about together: they turn up in the
// same short stretches of the video far more often than chance.
type Topic struct {
	Terms []string `json:"terms"` // most mentioned first
	Count int      `json:"count"` // comments mentioning any of them
	Hist  []int32  `json:"hist"`  // those comments over the video
	Peak  float64  `json:"peak"`  // second of the busiest stretch
}

// Topic detection parameters.
const (
	topicWindow  = 10.0 // seconds
	minCoWindows = 3
	minEdgeNPMI  = 0.3
	topNeighbors = 3 // an edge needs each end among the other's strongest
	maxTopics    = 8
)

// keyTerms lists, for each phrase, the terms it contains.
func (c *Corpus) keyTerms(terms []Term) [][]uint16 {
	out := make([][]uint16, len(c.Keys))
	var wg sync.WaitGroup
	for _, r := range shards(len(c.Keys)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := r[0]; k < r[1]; k++ {
				for t, term := range terms {
					if strings.Contains(c.Keys[k], term.Text) {
						out[k] = append(out[k], uint16(t))
					}
				}
			}
		}()
	}
	wg.Wait()
	return out
}

// bin is the time-distribution stretch of a moment.
func bin(vposMs int32, seconds float64) int {
	b := int(float64(vposMs) / 1000 / math.Max(seconds, 1) * HistBins)
	return max(0, min(HistBins-1, b))
}

// termHists fills each term's time distribution.
func (c *Corpus) termHists(terms []Term, kt [][]uint16, seconds float64) {
	for t := range terms {
		terms[t].Hist = make([]int32, HistBins)
	}
	for i, k := range c.Key {
		for _, t := range kt[k] {
			terms[t].Hist[bin(c.Vpos[i], seconds)]++
		}
	}
}

// Topics groups the terms into topics: a graph whose edges join terms
// found in the same 10-second stretches more often than chance (normalised
// PMI over stretches), cut into groups by weighted label propagation.
// Each term's Topic is set to its topic's index, or -1.
func (c *Corpus) Topics(terms []Term, kt [][]uint16, seconds float64) []Topic {
	n := len(terms)
	for t := range terms {
		terms[t].Topic = -1
	}
	if n < 2 {
		return nil
	}
	W := int(math.Ceil(math.Max(seconds, 1)/topicWindow)) + 1
	words := (n + 63) / 64
	present := make([]uint64, W*words)
	for i, k := range c.Key {
		w := int(float64(c.Vpos[i]) / 1000 / topicWindow)
		if w >= W {
			continue
		}
		for _, t := range kt[k] {
			present[w*words+int(t)/64] |= 1 << (t % 64)
		}
	}
	single := make([]float64, n)
	co := make([]float64, n*n)
	var ids []int
	for w := 0; w < W; w++ {
		ids = ids[:0]
		for t := 0; t < n; t++ {
			if present[w*words+t/64]&(1<<(t%64)) != 0 {
				ids = append(ids, t)
			}
		}
		for a, t := range ids {
			single[t]++
			for _, u := range ids[a+1:] {
				co[t*n+u]++
			}
		}
	}
	weight := make([][]float64, n)
	for t := range weight {
		weight[t] = make([]float64, n)
	}
	for t := 0; t < n; t++ {
		for u := t + 1; u < n; u++ {
			cc := co[t*n+u]
			if cc < minCoWindows {
				continue
			}
			pxy, px, py := cc/float64(W), single[t]/float64(W), single[u]/float64(W)
			if pxy >= 1 {
				continue
			}
			npmi := math.Log(pxy/(px*py)) / -math.Log(pxy)
			if npmi >= minEdgeNPMI {
				weight[t][u], weight[u][t] = npmi, npmi
			}
		}
	}
	// Mutual nearest neighbours only: common words linked weakly to many
	// topics would otherwise chain unrelated ones into one.
	top := make([]map[int]bool, n)
	for t := range weight {
		idx := make([]int, 0, n)
		for u, w := range weight[t] {
			if w > 0 {
				idx = append(idx, u)
			}
		}
		sort.Slice(idx, func(a, b int) bool { return weight[t][idx[a]] > weight[t][idx[b]] })
		top[t] = map[int]bool{}
		for _, u := range idx[:min(len(idx), topNeighbors)] {
			top[t][u] = true
		}
	}
	for t := range weight {
		for u := range weight[t] {
			if weight[t][u] > 0 && !(top[t][u] && top[u][t]) {
				weight[t][u] = 0
			}
		}
	}
	// Label propagation, in a fixed order so the result does not vary between runs.
	label := make([]int, n)
	for t := range label {
		label[t] = t
	}
	for round := 0; round < 30; round++ {
		changed := false
		for t := 0; t < n; t++ {
			score := map[int]float64{}
			for u, w := range weight[t] {
				if w > 0 {
					score[label[u]] += w
				}
			}
			best, bw := label[t], 0.0
			for l, w := range score {
				if w > bw || w == bw && l < best {
					best, bw = l, w
				}
			}
			if bw > 0 && best != label[t] {
				label[t], changed = best, true
			}
		}
		if !changed {
			break
		}
	}
	groups := map[int][]int{}
	for t, l := range label {
		groups[l] = append(groups[l], t)
	}
	type group struct {
		members []int
		count   int
	}
	var gs []group
	for _, m := range groups {
		if len(m) < 2 {
			continue
		}
		sort.Slice(m, func(i, j int) bool { return terms[m[i]].Count > terms[m[j]].Count })
		gs = append(gs, group{members: m})
	}
	// A comment counts once per topic, however many of its terms it has.
	member := make([]int, n)
	for t := range member {
		member[t] = -1
	}
	for g := range gs {
		for _, t := range gs[g].members {
			member[t] = g
		}
	}
	hists := make([][]int32, len(gs))
	for g := range hists {
		hists[g] = make([]int32, HistBins)
	}
	counts := make([]int, len(gs))
	seen := make([]int, len(gs))
	for g := range seen {
		seen[g] = -1
	}
	for i, k := range c.Key {
		for _, t := range kt[k] {
			if g := member[t]; g >= 0 && seen[g] != i {
				seen[g] = i
				counts[g]++
				hists[g][bin(c.Vpos[i], seconds)]++
			}
		}
	}
	for g := range gs {
		gs[g].count = counts[g]
	}
	order := make([]int, len(gs))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return gs[order[a]].count > gs[order[b]].count })
	var out []Topic
	for _, g := range order[:min(len(order), maxTopics)] {
		tp := Topic{Count: gs[g].count, Hist: hists[g]}
		for _, t := range gs[g].members[:min(len(gs[g].members), 8)] {
			tp.Terms = append(tp.Terms, terms[t].Text)
		}
		pk := 0
		for b, v := range hists[g] {
			if v > hists[g][pk] {
				pk = b
			}
		}
		tp.Peak = (float64(pk) + 0.5) / HistBins * seconds
		for _, t := range gs[g].members {
			terms[t].Topic = len(out)
		}
		out = append(out, tp)
	}
	return out
}
