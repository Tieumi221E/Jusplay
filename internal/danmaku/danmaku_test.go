package danmaku

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func TestFoldAndKey(t *testing.T) {
	for in, want := range map[string]string{
		"ＡＢＣ　ｄｅｆ！":      "abc def",
		"ｶﾞｯｷﾞﾂﾞﾃﾞﾄﾞﾊﾟ": "ガッギヅデドパ", // voicing marks joined, ッ not in the way
		"wwwwww":  "ww",
		"８８８８８８":  "88",
		"ああああ…":   "ああ",
		"公園に行くよ〜": "公園に行くよ",
		"隠したのさ!!": "隠したのさ",
		"!?":      "!?",
		"！？！？！？":  "!?!?",
		"は?":      "は?", // a short question keeps its mark
		"え…":      "え",
		"  ":      "",
	} {
		if got := Key(in); got != want {
			t.Errorf("Key(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLang(t *testing.T) {
	for in, want := range map[string]string{
		"かわいい": LangJa, "ヒカリノ博士": LangJa, "草": LangHan, "最高": LangHan, "这是什么": LangZh,
		"nice": LangEn, "ww": LangNone, "88": LangNone, "!?": LangNone, "대박": LangKo, "込み": LangJa,
	} {
		if got := Lang(in); got != want {
			t.Errorf("Lang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCategorize(t *testing.T) {
	for in, want := range map[string]Category{
		"ww": Laugh, "草": Laugh, "これは草": Laugh, "88": Applause, "おめでとう": Applause, "かわいい": Cute,
		"ここすき": Praise, "神回": Praise, "!?": Surprise, "は?": Surprise, "うわでた": Surprise,
		"泣いた": Sad, "かわいそう": Sad, "なんで": Tsukkomi, "そうはならんやろ": Tsukkomi, "うぽつ": Ritual,
		"見納め": Ritual, "公園に行くよ": Other, "ヒカリノ博士": Other,
		"それはそう": Agree, "せやな": Agree,
		// Once caught by unanchored patterns inside other words:
		"こっそり隠しておいたのさ": Other, "言ってくれなきゃわからない": Other, "神は死んだ": Other,
	} {
		if got := Categorize(Key(in), 0); got != want {
			t.Errorf("Categorize(%q) = %s, want %s", in, CategoryNames[got], CategoryNames[want])
		}
	}
	if Categorize("ww", Owner) != OwnerCat || Categorize("ww", Art) != ArtCat {
		t.Error("flags first")
	}
}

func TestHotspots(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	per := make([]int32, 1200)
	for i := range per {
		per[i] = int32(r.Intn(4)) // 0–3 a second
	}
	for s := 300; s < 306; s++ {
		per[s] += 40
	}
	for s := 900; s < 903; s++ {
		per[s] += 20
	}
	hs := Hotspots(per)
	if len(hs) < 2 || hs[0].Peak < 300 || hs[0].Peak > 306 || hs[1].Peak < 898 || hs[1].Peak > 904 {
		t.Fatalf("hotspots %+v", hs)
	}
	for _, h := range hs[2:] {
		t.Errorf("noise taken as a hotspot: %+v", h)
	}
	// Flat noise has none.
	flat := make([]int32, 1200)
	for i := range flat {
		flat[i] = int32(r.Intn(4))
	}
	if hs := Hotspots(flat); len(hs) != 0 {
		t.Errorf("flat: %+v", hs)
	}
}

func corpusOf(lines map[string]int) *Corpus {
	c := NewCorpus()
	i := 0
	for body, n := range lines {
		for k := 0; k < n; k++ {
			c.Add("main", &Comment{VposMs: int64(i%1000) * 1000, Body: body, UserID: fmt.Sprint("u", i%50)})
			i++
		}
	}
	return c
}

func TestTerms(t *testing.T) {
	// A name said inside many different lines is found whole; its parts
	// ("ミスティ", "小林") are not reported on their own.
	lines := map[string]int{}
	for i, s := range []string{"が来た", "かっこいい", "つよすぎ", "また出た", "だ!", "の声", "登場", "最高", "すき", "こわ"} {
		lines["ヒカリノ博士"+s] = 20 + i
		lines[s+"ヒカリノ博士"] = 15
	}
	for i := 0; i < 200; i++ {
		lines[fmt.Sprintf("ふつうのコメント%d", i)] = 1
	}
	c := corpusOf(lines)
	terms := c.Terms(20)
	if len(terms) == 0 || terms[0].Text != "ヒカリノ博士" {
		t.Fatalf("terms %+v", terms)
	}
	for _, x := range terms {
		if strings.Contains("ヒカリノ博士", x.Text) && x.Text != "ヒカリノ博士" {
			t.Errorf("part %q reported", x.Text)
		}
	}
}

func TestFamilies(t *testing.T) {
	c := corpusOf(map[string]int{
		"こっそり隠しておいたのさ": 60, "こっそり隠しておいたのさ!!": 50, "こっそり隠しておいたのさよ": 8, "こっそり隠しておいたんだ": 5,
		"ばっちぃ": 30, "ばっちい": 20, "やったぜ": 10, "やったか": 10,
		"公園に行くよ": 40, "公園に行くよ〜": 30, "かわいい": 100, "全然ちがう話": 3,
	})
	fs := c.Families(10)
	joined := false
	for _, f := range fs {
		if f.Top == "ばっちぃ" && len(f.Variants) == 1 && f.Variants[0] == "ばっちい" {
			joined = true
		}
	}
	if !joined {
		t.Errorf("ばっちぃ and ばっちい apart: %+v", fs)
	}
	if len(fs) == 0 || fs[0].Top != "こっそり隠しておいたのさ" || fs[0].Size < 3 {
		t.Fatalf("families %+v", fs)
	}
	for _, f := range fs {
		for _, v := range f.Variants {
			if v == "全然ちがう話" || v == "かわいい" || f.Top == "やったぜ" && v == "やったか" || f.Top == "やったか" && v == "やったぜ" {
				t.Errorf("%q joined %q", v, f.Top)
			}
		}
	}
}

func TestLoad(t *testing.T) {
	// Keys in any order; a comment before its thread's fork still gets it.
	src := `[{"comments":[{"vposMs":1500,"body":"ｗｗｗ","commands":["184","red","big"],"userId":"a","isPremium":true,"score":-2000,"postedAt":"2026-04-20T04:09:37+09:00","nicoruCount":3}],"fork":"owner","id":1},
	{"fork":"main","commentCount":2,"comments":[{"vposMs":2500,"body":"かわいい","commands":["ca"],"userId":"b","postedAt":"x"},{"vposMs":3000,"body":"","commands":[],"userId":"c"}]}]`
	c, err := Load(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if c.Len() != 2 || c.Forks[c.Fork[0]] != "owner" || c.Keys[c.Key[0]] != "ww" || c.Vpos[1] != 2500 {
		t.Fatalf("corpus %+v", c)
	}
	f := c.Flags[0]
	if f&(Anonymous|Colored|Big|Premium|Owner) != Anonymous|Colored|Big|Premium|Owner || c.Score[0] != -2000 || c.Nicoru[0] != 3 || c.Posted[0] == 0 {
		t.Errorf("first comment: flags %b score %d nicoru %d posted %d", f, c.Score[0], c.Nicoru[0], c.Posted[0])
	}
	if c.Flags[1]&Art == 0 || c.Posted[1] != 0 {
		t.Errorf("second comment: art %v posted %d", c.Flags[1]&Art != 0, c.Posted[1])
	}
	if _, err := Load(strings.NewReader(`{"not":"threads"}`)); err == nil {
		t.Error("an object accepted")
	}
}

func TestAnalyzeSmall(t *testing.T) {
	c := corpusOf(map[string]int{"ww": 30, "かわいい": 20, "公園に行くよ": 10, "なんで": 5})
	r := Analyze(c, 1000)
	if r.Comments != 65 || len(r.PerSecond) != 1000 || len(r.Categories) != int(numCategories) {
		t.Fatalf("report %+v", r)
	}
	got := map[string]int{}
	for _, x := range r.Categories {
		got[x.Name] = x.Count
	}
	if got["laugh"] != 30 || got["cute"] != 20 || got["tsukkomi"] != 5 || got["meme"] != 10 || got["other"] != 0 {
		t.Errorf("categories %v", got)
	}
	if r.Phrases[0].Text != "ww" || r.Phrases[0].Count != 30 {
		t.Errorf("phrases %+v", r.Phrases[:2])
	}
	if r.Users.Distinct != 50 {
		t.Errorf("users %+v", r.Users)
	}
}

func TestFragment(t *testing.T) {
	for s, want := range map[string]bool{"ティ": true, "ング": true, "ッド": true, "ーン": true, "上司": false, "世界": false, "ヒカリノ": false} {
		if got := fragment([]rune(s), 1.2); got != want {
			t.Errorf("fragment(%q) = %v", s, got)
		}
	}
	if !fragment([]rune("ちゃ"), 1.9) || fragment([]rune("ちゃ"), 3) {
		t.Error("two kana: a fragment unless free on both sides")
	}
}

func TestTopics(t *testing.T) {
	// Two groups talked about in different stretches, and a word said everywhere.
	c := NewCorpus()
	add := func(sec int, body string) {
		c.Add("main", &Comment{VposMs: int64(sec) * 1000, Body: body, UserID: "u"})
	}
	for rep := 0; rep < 6; rep++ {
		for s := 0; s < 600; s += 20 {
			add(s+rep, "いつもの挨拶")
		}
		for _, s := range []int{30, 90, 150, 210} {
			add(s+rep, "ヒカリノ博士が来た")
			add(s+rep+1, "ロボ助手もいる")
		}
		for _, s := range []int{330, 390, 450, 510} {
			add(s+rep, "海辺の町だ")
			add(s+rep+1, "灯台が見える")
		}
	}
	terms := []Term{{Text: "ヒカリノ博士", Count: 24}, {Text: "ロボ助手", Count: 24}, {Text: "海辺の町", Count: 24}, {Text: "灯台", Count: 24}, {Text: "いつもの挨拶", Count: 180}}
	kt := c.keyTerms(terms)
	tops := c.Topics(terms, kt, 600)
	if len(tops) != 2 {
		t.Fatalf("topics %+v", tops)
	}
	group := func(s string) int {
		for i, tp := range tops {
			for _, x := range tp.Terms {
				if x == s {
					return i
				}
			}
		}
		return -1
	}
	if group("ヒカリノ博士") != group("ロボ助手") || group("海辺の町") != group("灯台") || group("ヒカリノ博士") == group("灯台") {
		t.Errorf("groups %+v", tops)
	}
	if group("いつもの挨拶") != -1 || terms[4].Topic != -1 {
		t.Errorf("a word said everywhere joined a topic: %+v", tops)
	}
	if tops[0].Count != 48 || tops[0].Peak > 300 == (group("ヒカリノ博士") == 0) {
		t.Errorf("count or peak: %+v", tops)
	}
}
