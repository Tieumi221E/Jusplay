package filter

import (
	"encoding/json"
	"fmt"
	"testing"
)

// The cases of the page's former filter (web/test/filter.test.ts), with
// the same data and results.

var no int64

func c(body string, commands []string, edit ...func(*Comment)) Comment {
	no++
	x := Comment{ID: fmt.Sprintf("c%d", no), No: no, VposMs: no * 100, Body: body, Commands: commands, UserID: "u",
		PostedAt: "2026-01-02T10:00:00+09:00"}
	if x.Commands == nil {
		x.Commands = []string{}
	}
	for _, e := range edit {
		e(&x)
	}
	return x
}

func id(s string) json.RawMessage { b, _ := json.Marshal(s); return b }

func threads() []Thread {
	return []Thread{
		{ID: id("1"), Fork: "owner", Comments: []Comment{c("@デフォルト", []string{"red"}), c("投稿者 NG語", nil)}},
		{ID: id("2"), Fork: "main", Comments: []Comment{
			c("普通", nil), c("NG語を含む", nil), c("上", []string{"ue"}), c("下", []string{"shita", "big"}), c("赤", []string{"#ff0000"}),
			c("■■\n■■", []string{"ca", "small"}), c("匿名", []string{"184"}),
			c("x", nil, func(x *Comment) { x.UserID, x.Score, x.PostedAt = "bad", -5000, "2026-02-01T00:00:00+09:00" }),
		}},
		{ID: id("2"), Fork: "easy", Comments: []Comment{c("かんたん", nil)}},
	}
}

func run(edit func(*Settings)) Result {
	s := Defaults()
	s.CapPerSecond = 0 // the rule tests are about the other filters
	edit(&s)
	return Apply(threads(), s, 0)
}

func TestNoSettingsHidesNothing(t *testing.T) {
	r := run(func(*Settings) {})
	if r.Stats.Total != 11 || r.Stats.Shown != 11 || len(r.Keep) != 3 || len(r.Keep[1]) != 8 {
		t.Fatalf("%+v", r.Stats)
	}
}

func TestEachRule(t *testing.T) {
	r := run(func(s *Settings) {
		s.NgWords = []string{"NG語"}
		s.NgUsers = []string{"bad"}
		s.Hide.Ue, s.Hide.Colored, s.Hide.CA, s.Hide.Anonymous = true, true, true, true
		s.Forks["easy"] = false
	})
	want := map[string]int{"word": 1, "user": 1, "ue": 1, "colored": 1, "ca": 1, "anonymous": 1, "fork": 1}
	if fmt.Sprint(r.Stats.ByRule) != fmt.Sprint(want) {
		t.Fatalf("by rule %v", r.Stats.ByRule)
	}
	// Owner comments ignore viewer filters (投稿者 NG語 stays).
	if r.Stats.ByFork["owner"].Shown != 2 {
		t.Fatalf("owner shown %d", r.Stats.ByFork["owner"].Shown)
	}
}

func TestScriptsShareTimeCommandsRegex(t *testing.T) {
	if n := run(func(s *Settings) { s.RunScripts = false }).Stats.ByRule["script"]; n != 1 {
		t.Errorf("script %d", n)
	}
	if n := run(func(s *Settings) { s.NgShare = "medium" }).Stats.ByRule["ngShare"]; n != 1 {
		t.Errorf("ngShare medium %d", n)
	}
	if n, ok := run(func(s *Settings) { s.NgShare = "weak" }).Stats.ByRule["ngShare"]; ok {
		t.Errorf("ngShare weak %d", n)
	}
	if n := run(func(s *Settings) { s.PostedBefore = "2026-01-15" }).Stats.ByRule["time"]; n != 1 {
		t.Errorf("time %d", n)
	}
	if n := run(func(s *Settings) { s.NgCommands = []string{"BIG"} }).Stats.ByRule["command"]; n != 1 {
		t.Errorf("command %d", n)
	}
	if n := run(func(s *Settings) { s.NgWords = []string{"/^普/"} }).Stats.ByRule["word"]; n != 1 {
		t.Errorf("regex %d", n)
	}
	bad := run(func(s *Settings) { s.NgWords = []string{"/(/"} })
	if fmt.Sprint(bad.Stats.BadPatterns) != "[/(/]" || bad.Stats.Shown != 11 {
		t.Errorf("bad pattern %+v", bad.Stats)
	}
}

func TestHideScrolling(t *testing.T) {
	if n := run(func(s *Settings) { s.Hide.Naka = true }).Stats.ByRule["naka"]; n != 7 {
		t.Fatalf("naka %d", n)
	}
}

func TestCap(t *testing.T) {
	many := func() []Thread {
		m := Thread{ID: id("2"), Fork: "main"}
		for i := 0; i < 2000; i++ {
			body, cmds, user := fmt.Sprintf("m%d", i), []string{}, fmt.Sprintf("v%d", i)
			if i%100 == 0 {
				body, cmds, user = "■", []string{"ca"}, "artist"
			}
			m.Comments = append(m.Comments, Comment{ID: fmt.Sprintf("m-%d", i), No: int64(i), VposMs: int64(i * 50), Body: body, Commands: cmds, UserID: user, PostedAt: "2026-01-02T10:00:00+09:00"})
		}
		return []Thread{{ID: id("1"), Fork: "owner", Comments: []Comment{c("@デフォルト", nil), c("o1", nil), c("o2", nil)}}, m}
	}
	s := Defaults()
	s.CapPerSecond = 2 // 100 s video → limit 200
	a, b := Apply(many(), s, 100), Apply(many(), s, 100)
	if a.Stats.Cap.Limit != 200 {
		t.Fatalf("limit %d", a.Stats.Cap.Limit)
	}
	if d := a.Stats.Shown - 200; d < -30 || d > 30 {
		t.Fatalf("shown %d", a.Stats.Shown)
	}
	if fmt.Sprint(a.Keep) != fmt.Sprint(b.Keep) {
		t.Fatal("not stable")
	}
	if len(a.Keep[0]) != 3 {
		t.Fatal("owner comments capped")
	}
	th := many()[1]
	art, kept := 0, []Comment{}
	for _, i := range a.Keep[1] {
		if th.Comments[i].Commands != nil && len(th.Comments[i].Commands) > 0 {
			art++
		} else {
			kept = append(kept, th.Comments[i])
		}
	}
	if art != 20 {
		t.Fatalf("art kept %d", art)
	}
	// Density keeps its shape; kept ids are not clustered.
	for d := int64(0); d < 10; d++ {
		n := 0
		for _, x := range kept {
			if x.VposMs >= d*10000 && x.VposMs < (d+1)*10000 {
				n++
			}
		}
		if n < 8 || n > 30 {
			t.Errorf("tenth %d: %d", d, n)
		}
	}
	run, longest, prev := 0, 0, int64(-2)
	for _, x := range kept {
		if x.No == prev+1 {
			run++
		} else {
			run = 1
		}
		longest, prev = max(longest, run), x.No
	}
	if longest > 4 {
		t.Errorf("longest run %d", longest)
	}
	if _, ok := Apply(many(), s, 2000).Stats.ByRule["cap"]; ok {
		t.Error("capped under the budget")
	}
	s.CapPerSecond = 0
	if n := Apply(many(), s, 100).Stats.Shown; n != 2003 {
		t.Errorf("no cap: %d", n)
	}
	s.CapPerSecond = 0.5
	if l := Apply(many(), s, 10).Stats.Cap.Limit; l != 100 {
		t.Errorf("minimum %d", l)
	}
}

// Artists rarely use a "ca" command: the cap must keep every comment of a
// comment-art author, or the picture falls apart.
func TestCapKeepsArtAuthors(t *testing.T) {
	var cs []Comment
	for i := 0; i < 12; i++ {
		cs = append(cs, c(fmt.Sprintf("■□■□■□■□ %d", i), []string{"ue", "small", "full"}, func(x *Comment) { x.UserID, x.VposMs = "artist", 50000 }))
	}
	multi := c("■\n■\n■\n■", []string{"shita"}, func(x *Comment) { x.UserID, x.VposMs = "other", 60000 })
	cs = append(cs, multi)
	for i := 0; i < 400; i++ {
		i := i
		cs = append(cs, c(fmt.Sprintf("普通 %d", i), nil, func(x *Comment) { x.UserID, x.VposMs = fmt.Sprintf("v%d", i), int64(i*1000) }))
	}
	s := Defaults()
	s.CapPerSecond = 1
	r := Apply([]Thread{{ID: id("9"), Fork: "main", Comments: cs}}, s, 120)
	keep := map[int]bool{}
	for _, i := range r.Keep[0] {
		keep[i] = true
	}
	if r.Stats.ByRule["cap"] == 0 {
		t.Fatal("the cap is not in effect")
	}
	for i := 0; i < 13; i++ {
		if !keep[i] {
			t.Errorf("art line %d dropped", i)
		}
	}
}

func TestArtAndMaxLength(t *testing.T) {
	art := c("■□■□■□■□■□■□■□■□■□■□■□■□", []string{"ue", "ender"}, func(x *Comment) { x.UserID = "artist2" })
	plain := c("ふつうのながいコメントふつうのながいコメント", nil, func(x *Comment) { x.UserID = "p" })
	s := Defaults()
	s.CapPerSecond = 0
	s.MaxLength = 10
	if r := Apply([]Thread{{ID: id("9"), Fork: "main", Comments: []Comment{art, plain}}}, s, 0); fmt.Sprint(r.Keep) != "[[0]]" {
		t.Errorf("max length: %v", r.Keep)
	}
	s.MaxLength, s.Hide.CA = 0, true
	if r := Apply([]Thread{{ID: id("9"), Fork: "main", Comments: []Comment{art, plain}}}, s, 0); fmt.Sprint(r.Keep) != "[[1]]" {
		t.Errorf("hide art: %v", r.Keep)
	}
}

func TestFromDocument(t *testing.T) {
	s := FromDocument([]byte(`{"comments":{"forks":{"easy":false},"runScripts":false},"filters":{"ngWords":["a",3],"ngShare":"bogus","capPerSecond":99,"hide":{"ue":true,"x":true},"maxLength":-4}}`))
	if s.Forks["easy"] || !s.Forks["main"] || s.RunScripts || fmt.Sprint(s.NgWords) != "[a]" || s.NgShare != "none" || s.CapPerSecond != 50 || !s.Hide.Ue || s.MaxLength != 0 {
		t.Fatalf("%+v", s)
	}
	if d := FromDocument([]byte(`{}`)); d.CapPerSecond != 2 || !d.RunScripts || !d.Forks["owner"] {
		t.Fatalf("defaults %+v", d)
	}
}

// The hash must equal the page's: the same comments are kept under the
// cap, whichever side filtered (values computed with the page's fnv1a).
func TestHashAsThePage(t *testing.T) {
	for s, want := range map[string]float64{
		"2/main/m-1":  0.48274673824198544,
		"1/owner/c5":  0.6983837729785591,
		"9/main/c123": 0.8841323889791965,
		"2/main/弾幕😀":  0.5279227432329208, // a surrogate pair: UTF-16 code units, as the page
	} {
		if v := fnv1a(s); v != want {
			t.Errorf("%q: %v, the page gives %v", s, v, want)
		}
	}
}
