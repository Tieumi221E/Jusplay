package player

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tieumi221E/Jusplay/internal/embed"
)

func TestLoadCommentsOrder(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "Show - 01.mkv")
	b, err := os.ReadFile("../../testdata/media/tiny.mkv")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(video, b, 0o644)
	src, _ := os.ReadFile("../../testdata/zouryou.json")
	legacy, _ := os.ReadFile("../../testdata/legacy.xml")
	sameName := filepath.Join(dir, "Show - 01.json")
	other := filepath.Join(dir, "other.xml")
	broken := filepath.Join(dir, "broken.json")
	os.WriteFile(other, legacy, 0o644)
	os.WriteFile(broken, []byte("{nope"), 0o644)

	if c := LoadComments(video, ""); c.Source != "none" || c.Error != "" {
		t.Errorf("nothing: %+v", c)
	}
	os.WriteFile(sameName, src, 0o644)
	if c := LoadComments(video, ""); c.Source != "same-name" || c.playback == nil {
		t.Errorf("same-name: %+v", c)
	}
	if c := LoadComments(video, other); c.Source != "manual" || c.Path != other {
		t.Errorf("manual: %+v", c)
	}
	// A broken manual choice falls through, and says why.
	if c := LoadComments(video, broken); c.Source != "same-name" || len(c.Problems) != 1 || !strings.Contains(c.Problems[0], "手动") {
		t.Errorf("broken manual: %+v", c)
	}
	// Once embedded, the attachment wins over the same-name file.
	if _, err := embed.Embed(video, "", embed.Options{}); err != nil {
		t.Fatal(err)
	}
	if c := LoadComments(video, ""); c.Source != "attachment" || c.playback == nil {
		t.Errorf("attachment: %+v", c)
	}
	if c := LoadComments(video, other); c.Source != "manual" {
		t.Errorf("manual over attachment: %+v", c)
	}
}
