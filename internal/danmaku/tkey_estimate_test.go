package danmaku

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestTranslationWorkEstimate counts, for real comment files (bodies of the
// comments shown, one JSON array per file in JUSPLAY_CT_SHOWN, ";"), how many
// phrases would go to the model under each saving, one added at a time.
func TestTranslationWorkEstimate(t *testing.T) {
	list := os.Getenv("JUSPLAY_CT_SHOWN")
	if list == "" {
		t.Skip("JUSPLAY_CT_SHOWN not set")
	}
	for _, p := range strings.Split(list, ";") {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var bodies []string
		json.Unmarshal(b, &bodies)
		need := func(k string) bool { l := Lang(k); return l == LangJa || l == LangEn || l == LangKo }
		keys, cores, groups, afterDict := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
		comments := 0
		for _, body := range bodies {
			k := Key(body)
			if !need(k) {
				continue
			}
			comments++
			keys[k]++
			core, _, _ := TranslationCore(k)
			cores[core]++
			g := shapeFold(core)
			groups[g]++
			if _, ok := Reactions["zh"][g]; !ok {
				afterDict[g]++
			}
		}
		t.Logf("%s: %d shown comments need translating; phrases: exact %d → without laughter/marks %d → same shape %d → after the reaction table %d",
			p[strings.LastIndexAny(p, `/\`)+1:], comments, len(keys), len(cores), len(groups), len(afterDict))
	}
}
