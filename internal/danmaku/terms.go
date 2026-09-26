package danmaku

import (
	"math"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Term is a word or phrase found inside comments: a character sequence
// that holds together (its parts rarely occur apart: cohesion) and occurs
// in varied surroundings (high entropy of the character before and after).
// Names, catchphrases and in-jokes come out this way without a dictionary.
type Term struct {
	Text  string `json:"text"`
	Count int    `json:"count"` // comments containing it
	// Cohesion is the smallest normalised pointwise mutual information over
	// the ways to split it: log(p(xy)/(p(x)p(y))) / −log p(xy), in [−1, 1];
	// 1 when its parts never occur apart. Plain PMI is bounded by log(N/c),
	// so the most said phrases — a catchphrase in half the comments — would
	// score lowest; normalised, frequency does not count against them.
	Cohesion float64 `json:"cohesion"`
	Entropy  float64 `json:"entropy"` // min of left and right neighbour entropy (nats)
	Hist     []int32 `json:"hist,omitempty"`
	Topic    int     `json:"topic"` // index into Report.Topics, or -1
}

// Term discovery parameters.
const (
	minTermLen    = 2
	maxTermLen    = 8
	lossyEpsilon  = 2e-5 // Lossy Counting error, as a fraction of the comments seen
	maxCandidates = 6000
	minCohesion   = 0.45
	minEntropy    = 1.0
)

// FNV-1a, 64 bits: n-grams are counted by the hash of their bytes, grown
// one character at a time from each start, so no n-gram is copied, hashed
// from scratch or compared as a string. (A profile of a million comments
// showed string-keyed maps taking most of the time.) Collisions among the
// few million n-grams of even a large corpus are negligible at 64 bits.
const (
	fnvOffset = 14695981039346656037
	fnvPrime  = 1099511628211
)

func fnvString(s string) uint64 {
	h := uint64(fnvOffset)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= fnvPrime
	}
	return h
}

// lossy counts items approximately in bounded memory (Manku & Motwani's
// Lossy Counting): whenever the stream passes a multiple of 1/ε, entries
// whose count plus possible undercount is at most the bucket number are
// dropped. Any item with true count ≥ εN survives, undercounted by at most
// εN. Weighted: a phrase said k times adds k to each of its n-grams at once
// and advances the stream by k.
type lossy struct {
	w      int // bucket width, 1/ε
	n      int
	bucket int
	m      map[uint64]lossyEntry
}

type lossyEntry struct {
	count, delta int32
	s            string // the n-gram (a slice of its phrase)
}

func newLossy(eps float64) *lossy {
	return &lossy{w: int(math.Ceil(1 / eps)), bucket: 1, m: map[uint64]lossyEntry{}}
}

func (l *lossy) add(h uint64, s string, weight int) {
	e, ok := l.m[h]
	if ok {
		e.count += int32(weight)
	} else {
		e = lossyEntry{int32(weight), int32(l.bucket - 1), s}
	}
	l.m[h] = e
}

// next ends a unit of the given weight (a phrase and how often it is said).
func (l *lossy) next(weight int) {
	l.n += weight
	if b := l.n/l.w + 1; b > l.bucket {
		l.bucket = b
		for k, e := range l.m {
			if int(e.count+e.delta) <= l.bucket-1 {
				delete(l.m, k)
			}
		}
	}
}

// termRune reports whether a rune may be part of a term: letters and
// digits, the long vowel mark and the iteration mark; not marks or spaces.
func termRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == 'ー' || r == '々' || r == 'ヶ'
}

// gramsOf calls f for each n-gram of key (lengths lo..hi, term characters
// only) with its hash, its start and end byte offsets, and the characters
// around it (0 at a boundary). An n-gram said twice in one phrase is
// called twice; callers that need it once per phrase mark what they saw.
func gramsOf(key string, lo, hi int, f func(h uint64, a, b int, left, right rune)) {
	var prev rune
	for a, r0 := range key {
		if !termRune(r0) {
			prev = r0
			continue
		}
		h := uint64(fnvOffset)
		b, n := a, 0
		for b < len(key) && n < hi {
			r, size := utf8.DecodeRuneInString(key[b:])
			if !termRune(r) {
				break
			}
			for j := b; j < b+size; j++ {
				h ^= uint64(key[j])
				h *= fnvPrime
			}
			b += size
			n++
			if n >= lo {
				var right rune
				if b < len(key) {
					right, _ = utf8.DecodeRuneInString(key[b:])
				}
				f(h, a, b, prev, right)
			}
		}
		prev = r0
	}
}

// shards splits the phrases into one contiguous range per CPU.
func shards(n int) [][2]int {
	p := max(1, min(runtime.GOMAXPROCS(0), n/1000+1))
	out := make([][2]int, 0, p)
	for i := 0; i < p; i++ {
		out = append(out, [2]int{n * i / p, n * (i + 1) / p})
	}
	return out
}

// Terms discovers words and phrases in the comments, in two passes over
// the distinct phrases (weighted by how many comments say them), each
// split across the CPUs:
//
//  1. Lossy Counting of every 2–8 character n-gram finds the frequent ones
//     in bounded memory. Each shard counts its phrases with the same ε;
//     the shards' errors add up to at most εN, the guarantee of one pass.
//  2. For the most frequent candidates, exact counts of them and of their
//     parts, once per comment, and of the characters on each side.
//
// A candidate is kept when it holds together (cohesion) and is free on
// both sides (entropy), is not a piece of a word (fragment), and is not
// just part of a longer term that occurs almost as often ("ヒカリノ博"
// inside "ヒカリノ博士").
func (c *Corpus) Terms(limit int) []Term {
	N := c.Len()
	if N == 0 {
		return nil
	}
	minCount := max(5, N/5000)
	parts := shards(len(c.Keys))
	// Pass 1, split by n-gram, not by phrase: every worker reads all the
	// phrases but counts only the n-grams whose hash falls to it, so each
	// n-gram is in one table and memory stays that of a single pass (split
	// by phrase, 16 tables each held their own copy: 1.2 GB instead of 0.5
	// for a million comments). Each n-gram's whole stream is in its worker,
	// so the Lossy Counting guarantee holds as for one pass.
	workers := uint64(len(parts))
	counters := make([]*lossy, workers)
	var wg sync.WaitGroup
	for p := range counters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lc := newLossy(lossyEpsilon)
			for k, key := range c.Keys {
				times := int(c.KeyCount[k])
				if times == 0 {
					continue
				}
				gramsOf(key, minTermLen, maxTermLen, func(h uint64, a, b int, _, _ rune) {
					if h%workers == uint64(p) {
						lc.add(h, key[a:b], times)
					}
				})
				lc.next(times)
			}
			counters[p] = lc
		}()
	}
	wg.Wait()
	merged := map[uint64]lossyEntry{}
	for _, lc := range counters {
		for h, e := range lc.m {
			m := merged[h]
			m.count += e.count
			m.s = e.s
			merged[h] = m
		}
	}
	type cand struct {
		g string
		n int32
	}
	var cs []cand
	for _, e := range merged {
		if int(e.count) >= minCount {
			cs = append(cs, cand{e.s, e.count})
		}
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].n > cs[j].n || cs[i].n == cs[j].n && cs[i].g < cs[j].g })
	if len(cs) > maxCandidates {
		cs = cs[:maxCandidates]
	}
	// Pass 2: the candidates and all their parts get an index; each shard
	// counts into its own arrays, marking per phrase what it saw.
	index := map[uint64]int{}
	var texts []string
	want := func(s string) int {
		h := fnvString(s)
		if i, ok := index[h]; ok {
			return i
		}
		index[h] = len(texts)
		texts = append(texts, s)
		return len(texts) - 1
	}
	candIdx := make([]int, len(cs))
	isCand := map[int]int{} // index → candidate number
	for j, x := range cs {
		for i := range x.g {
			if i > 0 {
				want(x.g[:i])
				want(x.g[i:])
			}
		}
		candIdx[j] = want(x.g)
		isCand[candIdx[j]] = j
	}
	type tally struct {
		count       []int
		left, right []map[rune]int
		bound       [][2]int
	}
	tallies := make([]tally, len(parts))
	for p, r := range parts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t := tally{count: make([]int, len(texts)), left: make([]map[rune]int, len(cs)), right: make([]map[rune]int, len(cs)), bound: make([][2]int, len(cs))}
			mark := make([]int32, len(texts))
			for k := r[0]; k < r[1]; k++ {
				times := int(c.KeyCount[k])
				if times == 0 {
					continue
				}
				gramsOf(c.Keys[k], 1, maxTermLen, func(h uint64, _, _ int, l, rr rune) {
					i, ok := index[h]
					if !ok || mark[i] == int32(k)+1 {
						return
					}
					mark[i] = int32(k) + 1
					t.count[i] += times
					j, ok := isCand[i]
					if !ok {
						return
					}
					if l == 0 || !termRune(l) {
						t.bound[j][0] += times
					} else {
						if t.left[j] == nil {
							t.left[j] = map[rune]int{}
						}
						t.left[j][l] += times
					}
					if rr == 0 || !termRune(rr) {
						t.bound[j][1] += times
					} else {
						if t.right[j] == nil {
							t.right[j] = map[rune]int{}
						}
						t.right[j][rr] += times
					}
				})
			}
			tallies[p] = t
		}()
	}
	wg.Wait()
	count := make([]int, len(texts))
	left := make([]map[rune]int, len(cs))
	right := make([]map[rune]int, len(cs))
	bound := make([][2]int, len(cs))
	for _, t := range tallies {
		for i, n := range t.count {
			count[i] += n
		}
		for j := range cs {
			bound[j][0] += t.bound[j][0]
			bound[j][1] += t.bound[j][1]
			left[j] = addRunes(left[j], t.left[j])
			right[j] = addRunes(right[j], t.right[j])
		}
	}
	countOf := func(s string) int {
		if i, ok := index[fnvString(s)]; ok {
			return count[i]
		}
		return 0
	}
	var out []Term
	for j, x := range cs {
		cnt := count[candIdx[j]]
		if cnt < minCount {
			continue
		}
		coh := math.Inf(1)
		for i := range x.g {
			if i == 0 {
				continue
			}
			a, b := countOf(x.g[:i]), countOf(x.g[i:])
			if a == 0 || b == 0 {
				continue
			}
			pxy := float64(cnt) / float64(N)
			pmi := math.Log(pxy / (float64(a) / float64(N) * float64(b) / float64(N)))
			npmi := 1.0
			if pxy < 1 {
				npmi = pmi / -math.Log(pxy)
			}
			coh = math.Min(coh, npmi)
		}
		ent := math.Min(entropy(left[j], bound[j][0]), entropy(right[j], bound[j][1]))
		if coh < minCohesion || ent < minEntropy || fragment([]rune(x.g), ent) {
			continue
		}
		out = append(out, Term{Text: x.g, Count: cnt, Cohesion: round2(coh), Entropy: round2(ent)})
	}
	// Drop a term that is part of a longer kept term occurring nearly as often.
	sort.Slice(out, func(i, j int) bool { return len(out[i].Text) > len(out[j].Text) })
	var kept []Term
	for _, t := range out {
		shadowed := false
		for _, k := range kept {
			if k.Count >= t.Count*8/10 && strings.Contains(k.Text, t.Text) {
				shadowed = true
				break
			}
		}
		if !shadowed {
			kept = append(kept, t)
		}
	}
	sort.Slice(kept, func(i, j int) bool {
		return kept[i].Count > kept[j].Count || kept[i].Count == kept[j].Count && kept[i].Text < kept[j].Text
	})
	if len(kept) > limit {
		kept = kept[:limit]
	}
	return kept
}

func addRunes(dst, src map[rune]int) map[rune]int {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = map[rune]int{}
	}
	for r, n := range src {
		dst[r] += n
	}
	return dst
}

// entropy of the neighbours on one side. Each boundary occurrence counts
// as a neighbour of its own: a word said on its own is maximally free on
// that side.
func entropy(m map[rune]int, boundary int) float64 {
	total := boundary
	for _, n := range m {
		total += n
	}
	if total == 0 {
		return 0
	}
	var h float64
	for _, n := range m {
		p := float64(n) / float64(total)
		h -= p * math.Log(p)
	}
	if boundary > 0 {
		p := 1 / float64(total)
		h -= float64(boundary) * p * math.Log(p)
	}
	return h
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }

// fragment tells a piece of a word from a word. On real comments the
// candidates that passed cohesion and entropy but were not words were
// katakana pieces starting where no word starts ("ティ", "ング", "ッド":
// a small kana, the long vowel mark, ン) and two-kana pieces of inflection
// ("ちゃ", "じゃ", "すぎ"), which a real two-kana word clears by having
// many different neighbours. Two Chinese characters ("上司", "世界") are
// words however few their neighbours.
func fragment(rs []rune, entropy float64) bool {
	digits := true
	for _, r := range rs {
		if r < '0' || r > '9' {
			digits = false
		}
	}
	if digits {
		return true
	}
	if strings.ContainsRune("ァィゥェォッャュョヮヵヶぁぃぅぇぉっゃゅょゎーンん", rs[0]) {
		return true
	}
	kana := true
	for _, r := range rs {
		if !unicode.In(r, unicode.Hiragana, unicode.Katakana) {
			kana = false
		}
	}
	return kana && len(rs) <= 2 && entropy < 2.5
}
