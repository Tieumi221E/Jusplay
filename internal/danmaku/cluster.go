package danmaku

import (
	"hash/fnv"
	"sort"
	"strings"
)

// Family is a group of phrases that are writings of the same line: a
// catchphrase typed with a word changed, a mark added, a letter missed.
type Family struct {
	Top      string   `json:"top"`      // the most said writing
	Count    int      `json:"count"`    // comments in the family
	Variants []string `json:"variants"` // other writings, most said first (at most 5)
	Size     int      `json:"size"`     // distinct writings
}

// Family detection parameters.
const (
	minHashes  = 16  // MinHash signature length
	bandRows   = 2   // rows per LSH band: 8 bands, catching Jaccard ≳ 0.5
	minJaccard = 0.6 // 0.5 joined やったぜ with やったか (2 of 4 bigrams shared)
	maxBucket  = 400 // larger buckets (very common bigrams) are skipped
	minFamLen  = 3   // runes: shorter phrases are their own family
)

// Families groups the phrases said at least twice into families of near
// writings: MinHash signatures of their character bigrams, locality
// sensitive hashing into buckets by bands of the signature, and within a
// bucket an exact Jaccard check. Short texts make SimHash unreliable;
// MinHash estimates the bigram Jaccard similarity directly. No pair is
// compared outside a shared bucket, so the work grows with the number of
// phrases, not its square.
func (c *Corpus) Families(limit int) []Family {
	var ids []int
	for k, n := range c.KeyCount {
		if n >= 2 && len([]rune(c.Keys[k])) >= minFamLen {
			ids = append(ids, k)
		}
	}
	bigr := make(map[int]map[string]struct{}, len(ids))
	sig := make(map[int][minHashes]uint32, len(ids))
	for _, k := range ids {
		rs := []rune(shapeFold(c.Keys[k]))
		set := map[string]struct{}{}
		for i := 0; i+1 < len(rs); i++ {
			set[string(rs[i:i+2])] = struct{}{}
		}
		bigr[k] = set
		var s [minHashes]uint32
		for i := range s {
			s[i] = ^uint32(0)
		}
		for g := range set {
			h := fnv.New64a()
			h.Write([]byte(g))
			x := h.Sum64()
			a, b := uint32(x), uint32(x>>32)
			for i := range s {
				// Double hashing: h_i = a + i·b gives independent-enough permutations.
				if v := a + uint32(i)*b; v < s[i] {
					s[i] = v
				}
			}
		}
		sig[k] = s
	}
	parent := map[int]int{}
	var find func(int) int
	find = func(x int) int {
		p, ok := parent[x]
		if !ok || p == x {
			return x
		}
		r := find(p)
		parent[x] = r
		return r
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}
	jaccard := func(a, b map[string]struct{}) float64 {
		inter := 0
		for g := range a {
			if _, ok := b[g]; ok {
				inter++
			}
		}
		u := len(a) + len(b) - inter
		if u == 0 {
			return 0
		}
		return float64(inter) / float64(u)
	}
	for band := 0; band < minHashes/bandRows; band++ {
		buckets := map[[bandRows]uint32][]int{}
		for _, k := range ids {
			var key [bandRows]uint32
			s := sig[k]
			copy(key[:], s[band*bandRows:])
			buckets[key] = append(buckets[key], k)
		}
		for _, b := range buckets {
			if len(b) < 2 || len(b) > maxBucket {
				continue
			}
			for i := 0; i < len(b); i++ {
				for j := i + 1; j < len(b); j++ {
					if find(b[i]) != find(b[j]) && jaccard(bigr[b[i]], bigr[b[j]]) >= minJaccard {
						union(b[i], b[j])
					}
				}
			}
		}
	}
	groups := map[int][]int{}
	for _, k := range ids {
		r := find(k)
		groups[r] = append(groups[r], k)
	}
	var out []Family
	for _, g := range groups {
		if len(g) < 2 {
			continue
		}
		sort.Slice(g, func(i, j int) bool { return c.KeyCount[g[i]] > c.KeyCount[g[j]] })
		f := Family{Top: c.Keys[g[0]], Size: len(g)}
		for i, k := range g {
			f.Count += int(c.KeyCount[k])
			if i > 0 && len(f.Variants) < 5 {
				f.Variants = append(f.Variants, c.Keys[k])
			}
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Count > out[j].Count || out[i].Count == out[j].Count && out[i].Top < out[j].Top
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// shapeFold is a phrase as compared for families (not as shown): katakana
// as hiragana, small kana as large, long vowel marks and wave dashes gone.
// "ばっちぃ" and "ばっちい", "カワイイ" and "かわいい" then share all their
// bigrams, and the Jaccard threshold can stay high enough to keep
// "やったぜ" apart from "やったか".
func shapeFold(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'ァ' && r <= 'ヶ' {
			r -= 'ァ' - 'ぁ'
		}
		if big, ok := smallKana[r]; ok {
			r = big
		}
		if r == 'ー' || r == '〜' || r == '~' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

var smallKana = map[rune]rune{'ぁ': 'あ', 'ぃ': 'い', 'ぅ': 'う', 'ぇ': 'え', 'ぉ': 'お', 'ゃ': 'や', 'ゅ': 'ゆ', 'ょ': 'よ', 'ゎ': 'わ'}
