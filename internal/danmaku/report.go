package danmaku

import (
	"math"
	"sort"
	"time"
	"unicode/utf8"
)

// Report is everything the analysis finds, ready for the page.
type Report struct {
	Comments int     `json:"comments"`
	Distinct int     `json:"distinct"` // distinct phrases (after Key)
	Duration float64 `json:"duration"` // seconds of video covered

	// Per second of video, and per bucket the mix of categories.
	PerSecond  []int32    `json:"perSecond"`
	BucketSec  int        `json:"bucketSec"`
	CatBuckets [][]int32  `json:"catBuckets"` // [bucket][category]
	Hotspots   []HotspotR `json:"hotspots"`

	Categories []CatCount `json:"categories"` // all categories, with Other: coverage is 1 − other/comments
	Languages  []Count    `json:"languages"`
	Phrases    []Phrase   `json:"phrases"`  // most said whole comments
	Terms      []Term     `json:"terms"`    // words and phrases found inside comments
	Families   []Family   `json:"families"` // groups of writings of the same line
	Topics     []Topic    `json:"topics"`

	Users   UserStats          `json:"users"`
	Posting PostStats          `json:"posting"`
	Style   []Count            `json:"style"` // commands: anonymous, colour, size, position, device, art
	Forks   []Count            `json:"forks"`
	Length  []Count            `json:"length"` // characters per comment, bucketed
	Nicoru  []Liked            `json:"nicoru"` // most ニコる'd comments
	NGScore []Count            `json:"ngScore"`
	Timing  map[string]float64 `json:"timingMs"` // how long each part took
}

// HotspotR is a hotspot with what was said there.
type HotspotR struct {
	Hotspot
	Phrases []Count `json:"phrases"` // said there far more than elsewhere
	Mix     []Count `json:"mix"`     // categories there
}

// Count is a named number.
type Count struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// CatCount is a category with its most said phrases.
type CatCount struct {
	Name    string  `json:"name"`
	Count   int     `json:"count"`
	Phrases []Count `json:"phrases"`
}

// Phrase is a whole comment as often said.
type Phrase struct {
	Text     string  `json:"text"`
	Count    int     `json:"count"`
	Category string  `json:"category"`
	Lang     string  `json:"lang"`
	Peak     float64 `json:"peak"`   // second with most of it
	Spread   float64 `json:"spread"` // share said within ±10 s of the peak: near 1, a moment's line; low, a running reaction
	Hist     []int32 `json:"hist"`
}

// UserStats describes who comments (never who: no ids leave the analysis).
type UserStats struct {
	Distinct  int     `json:"distinct"`
	Top1Share float64 `json:"top1Share"`
	Top10     float64 `json:"top10Share"` // share of comments by the 10 most active accounts
	Gini      float64 `json:"gini"`
	Median    float64 `json:"median"` // comments per account
	Anonymous float64 `json:"anonymousShare"`
	Premium   float64 `json:"premiumShare"`
}

// PostStats describes when comments were posted (not where in the video).
type PostStats struct {
	First, Median, Last string  `json:",omitempty"`
	FirstDayShare       float64 `json:"firstDayShare"` // posted within 24 h of the first
	Days                []Count `json:"days"`          // per day since the first (at most 400)
	Hours               []int   `json:"hours"`         // per hour of day (JST), 24 values
}

// Liked is a comment many liked.
type Liked struct {
	Text   string  `json:"text"`
	At     float64 `json:"at"`
	Nicoru int     `json:"nicoru"`
}

// Analyze runs every analysis. seconds is the video's length (0: from the
// comments).
func Analyze(c *Corpus, seconds float64) *Report {
	timing := map[string]float64{}
	lap := time.Now()
	mark := func(name string) {
		timing[name] = float64(time.Since(lap).Microseconds()) / 1000
		lap = time.Now()
	}
	N := c.Len()
	r := &Report{Comments: N, Distinct: len(c.Keys), Timing: timing}
	secs := int(math.Ceil(seconds))
	r.PerSecond = c.Timeline(secs)
	r.Duration = float64(len(r.PerSecond))
	r.Terms = c.Terms(80)
	mark("terms")
	kt := c.keyTerms(r.Terms)
	c.termHists(r.Terms, kt, r.Duration)
	r.Topics = c.Topics(r.Terms, kt, r.Duration)
	mark("topics")
	// Per comment: category (from its key, flags) and language (per key).
	// What the reaction rules do not know gets a second look: a line many
	// repeat is a meme, one naming a term found (a character, a place) a
	// mention, a sentence of its own commentary.
	named := make([]bool, len(r.Terms))
	for t, term := range r.Terms {
		named[t] = term.Topic >= 0 || hasKatakana(term.Text)
	}
	memeMin := uint32(max(5, N/2000))
	second := func(k uint32) Category {
		if c.KeyCount[k] >= memeMin {
			return Meme
		}
		for _, t := range kt[k] {
			if named[t] {
				return Mention
			}
		}
		if utf8.RuneCountInString(c.Keys[k]) >= commentaryLen {
			return Commentary
		}
		return Other
	}
	cats := make([]Category, N)
	keyCat := make(map[uint32]Category, len(c.Keys))
	keyLang := make([]string, len(c.Keys))
	for k, key := range c.Keys {
		keyLang[k] = Lang(key)
	}
	var catCount [numCategories]int
	for i := 0; i < N; i++ {
		var cat Category
		if c.Flags[i]&(Owner|Art) != 0 {
			cat = Categorize("", c.Flags[i])
		} else if v, ok := keyCat[c.Key[i]]; ok {
			cat = v
		} else {
			cat = Categorize(c.Keys[c.Key[i]], 0)
			if cat == Other {
				cat = second(c.Key[i])
			}
			keyCat[c.Key[i]] = cat
		}
		cats[i] = cat
		catCount[cat]++
	}
	mark("categories")
	// Timeline buckets: about 150 across the video.
	r.BucketSec = max(5, int(r.Duration)/150)
	nb := int(r.Duration)/r.BucketSec + 1
	r.CatBuckets = make([][]int32, nb)
	for b := range r.CatBuckets {
		r.CatBuckets[b] = make([]int32, numCategories)
	}
	for i := 0; i < N; i++ {
		if b := int(c.Vpos[i]/1000) / r.BucketSec; b < nb {
			r.CatBuckets[b][cats[i]]++
		}
	}
	// Phrases: per key its count and where it peaks.
	order := make([]int, 0, len(c.Keys))
	for k := range c.Keys {
		if c.KeyCount[k] >= 2 && c.Keys[k] != "" {
			order = append(order, k)
		}
	}
	sort.Slice(order, func(i, j int) bool { return c.KeyCount[order[i]] > c.KeyCount[order[j]] })
	top := order[:min(len(order), 200)]
	topSet := map[uint32]int{}
	for i, k := range top {
		topSet[uint32(k)] = i
	}
	secsOf := make([][]int32, len(top))
	for i := 0; i < N; i++ {
		if j, ok := topSet[c.Key[i]]; ok {
			secsOf[j] = append(secsOf[j], c.Vpos[i]/1000)
		}
	}
	for j, k := range top {
		peak, spread := peakOf(secsOf[j])
		hist := make([]int32, HistBins)
		for _, s := range secsOf[j] {
			hist[bin(s*1000, r.Duration)]++
		}
		r.Phrases = append(r.Phrases, Phrase{Text: c.Keys[k], Count: int(c.KeyCount[k]), Category: CategoryNames[keyCatOf(c, keyCat, k)],
			Lang: keyLang[k], Peak: peak, Spread: round2(spread), Hist: hist})
	}
	if len(r.Phrases) > 100 {
		r.Phrases = r.Phrases[:100]
	}
	mark("phrases")
	for cat := Category(0); cat < numCategories; cat++ {
		cc := CatCount{Name: CategoryNames[cat], Count: catCount[cat]}
		for _, k := range order {
			if len(cc.Phrases) == 5 {
				break
			}
			if keyCatOf(c, keyCat, k) == cat && cat != ArtCat && cat != OwnerCat {
				cc.Phrases = append(cc.Phrases, Count{c.Keys[k], int(c.KeyCount[k])})
			}
		}
		r.Categories = append(r.Categories, cc)
	}
	langs := map[string]int{}
	for i := 0; i < N; i++ {
		langs[keyLang[c.Key[i]]]++
	}
	r.Languages = sorted(langs)
	// Hotspots, with the phrases said there far more than elsewhere.
	for _, h := range Hotspots(r.PerSecond) {
		hr := HotspotR{Hotspot: h}
		in := map[uint32]int{}
		mix := map[string]int{}
		a, b := int32((h.Start-2)*1000), int32((h.End+2)*1000)
		for i := 0; i < N; i++ {
			if c.Vpos[i] >= a && c.Vpos[i] < b {
				in[c.Key[i]]++
				mix[CategoryNames[cats[i]]]++
			}
		}
		width := float64(b-a) / 1000
		type sc struct {
			k uint32
			n int
			s float64
		}
		var ss []sc
		for k, n := range in {
			if n < 3 || c.Keys[k] == "" {
				continue
			}
			expected := float64(c.KeyCount[k]) * width / math.Max(r.Duration, 1)
			lift := float64(n) / math.Max(expected, 1e-9)
			ss = append(ss, sc{k, n, float64(n) * math.Log(math.Max(lift, 1))})
		}
		sort.Slice(ss, func(i, j int) bool { return ss[i].s > ss[j].s })
		for _, s := range ss[:min(len(ss), 5)] {
			hr.Phrases = append(hr.Phrases, Count{c.Keys[s.k], s.n})
		}
		hr.Mix = sorted(mix)
		hr.Z = round2(hr.Z)
		hr.Rate, hr.Baseline = round2(hr.Rate), round2(hr.Baseline)
		r.Hotspots = append(r.Hotspots, hr)
	}
	mark("hotspots")
	r.Families = c.Families(40)
	mark("families")
	r.Users = c.userStats()
	r.Posting = c.postStats()
	r.Style, r.Forks, r.Length, r.NGScore = c.styleStats()
	r.Nicoru = c.liked(10)
	mark("stats")
	return r
}

// commentaryLen is how many characters make a line a sentence of its own.
const commentaryLen = 12

func hasKatakana(s string) bool {
	for _, r := range s {
		if r >= 'ァ' && r <= 'ヺ' {
			return true
		}
	}
	return false
}

func keyCatOf(c *Corpus, m map[uint32]Category, k int) Category {
	if v, ok := m[uint32(k)]; ok {
		return v
	}
	return Categorize(c.Keys[k], 0)
}

// peakOf is the second with the most occurrences and the share within ±10 s of it.
func peakOf(s []int32) (float64, float64) {
	if len(s) == 0 {
		return 0, 0
	}
	cnt := map[int32]int{}
	for _, x := range s {
		cnt[x]++
	}
	best, bn := int32(0), -1
	for x := range cnt {
		// The densest 21 s window around each second.
		w := 0
		for d := int32(-10); d <= 10; d++ {
			w += cnt[x+d]
		}
		if w > bn || w == bn && x < best {
			best, bn = x, w
		}
	}
	return float64(best), float64(bn) / float64(len(s))
}

func sorted(m map[string]int) []Count {
	out := make([]Count, 0, len(m))
	for k, v := range m {
		out = append(out, Count{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Count > out[j].Count || out[i].Count == out[j].Count && out[i].Name < out[j].Name
	})
	return out
}

func (c *Corpus) userStats() UserStats {
	N := c.Len()
	per := make([]int, len(c.Users))
	anon, prem := 0, 0
	for i := 0; i < N; i++ {
		per[c.User[i]]++
		if c.Flags[i]&Anonymous != 0 {
			anon++
		}
		if c.Flags[i]&Premium != 0 {
			prem++
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(per)))
	u := UserStats{Distinct: len(per)}
	if N == 0 || len(per) == 0 {
		return u
	}
	t10 := 0
	for _, n := range per[:min(10, len(per))] {
		t10 += n
	}
	u.Top1Share = round3(float64(per[0]) / float64(N))
	u.Top10 = round3(float64(t10) / float64(N))
	u.Median = float64(per[len(per)/2])
	// Gini over accounts: 1 − 2·Σ (cumulative share) / n, ascending order.
	var cum, area float64
	for i := len(per) - 1; i >= 0; i-- {
		cum += float64(per[i])
		area += cum
	}
	n := float64(len(per))
	u.Gini = round3(1 - 2*area/(n*float64(N)) + 1/n)
	u.Anonymous = round3(float64(anon) / float64(N))
	u.Premium = round3(float64(prem) / float64(N))
	return u
}

var jst = time.FixedZone("JST", 9*3600)

func (c *Corpus) postStats() PostStats {
	var ts []int64
	for _, t := range c.Posted {
		if t > 0 {
			ts = append(ts, t)
		}
	}
	p := PostStats{Hours: make([]int, 24)}
	if len(ts) == 0 {
		return p
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i] < ts[j] })
	f := func(t int64) string { return time.Unix(t, 0).In(jst).Format(time.RFC3339) }
	p.First, p.Median, p.Last = f(ts[0]), f(ts[len(ts)/2]), f(ts[len(ts)-1])
	day := map[int]int{}
	within := 0
	for _, t := range ts {
		d := int((t - ts[0]) / 86400)
		day[d]++
		if t-ts[0] < 86400 {
			within++
		}
		p.Hours[time.Unix(t, 0).In(jst).Hour()]++
	}
	p.FirstDayShare = round3(float64(within) / float64(len(ts)))
	last := int((ts[len(ts)-1] - ts[0]) / 86400)
	for d := 0; d <= min(last, 399); d++ {
		p.Days = append(p.Days, Count{Name: time.Unix(ts[0]+int64(d)*86400, 0).In(jst).Format("2006-01-02"), Count: day[d]})
	}
	return p
}

func (c *Corpus) styleStats() (style, forks, length, ng []Count) {
	names := []struct {
		f uint16
		n string
	}{{Anonymous, "anonymous"}, {Colored, "colored"}, {Big, "big"}, {Small, "small"}, {Top, "ue"}, {Bottom, "shita"}, {Switch, "switch"}, {Art, "art"}}
	cnt := make([]int, len(names))
	fk := map[string]int{}
	lb := []struct {
		max int
		n   string
	}{{2, "1-2"}, {5, "3-5"}, {10, "6-10"}, {20, "11-20"}, {40, "21-40"}, {1 << 30, "41+"}}
	lc := make([]int, len(lb))
	ngb := []struct {
		max int32
		n   string
	}{{-10000, "≤-10000"}, {-4800, "-9999…-4800"}, {-1000, "-4799…-1000"}, {-1, "-999…-1"}, {math.MaxInt32, "≥0"}}
	nc := make([]int, len(ngb))
	for i := range c.Vpos {
		for j, x := range names {
			if c.Flags[i]&x.f != 0 {
				cnt[j]++
			}
		}
		fk[c.Forks[c.Fork[i]]]++
		n := utf8.RuneCountInString(c.Bodies[c.Body[i]])
		for j, b := range lb {
			if n <= b.max {
				lc[j]++
				break
			}
		}
		for j, b := range ngb {
			if c.Score[i] <= b.max {
				nc[j]++
				break
			}
		}
	}
	for j, x := range names {
		style = append(style, Count{x.n, cnt[j]})
	}
	for j, b := range lb {
		length = append(length, Count{b.n, lc[j]})
	}
	for j, b := range ngb {
		ng = append(ng, Count{b.n, nc[j]})
	}
	return style, sorted(fk), length, ng
}

func (c *Corpus) liked(n int) []Liked {
	idx := make([]int, 0, c.Len())
	for i, v := range c.Nicoru {
		if v > 0 {
			idx = append(idx, i)
		}
	}
	sort.Slice(idx, func(a, b int) bool { return c.Nicoru[idx[a]] > c.Nicoru[idx[b]] })
	var out []Liked
	for _, i := range idx[:min(n, len(idx))] {
		out = append(out, Liked{Text: c.Bodies[c.Body[i]], At: float64(c.Vpos[i]) / 1000, Nicoru: int(c.Nicoru[i])})
	}
	return out
}

func round3(x float64) float64 { return math.Round(x*1000) / 1000 }
