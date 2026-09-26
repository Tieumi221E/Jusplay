package subs

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Formats are the subtitle file types the page reads.
var Formats = map[string]string{".srt": "srt", ".ass": "ass", ".ssa": "ass", ".vtt": "vtt"}

// File is a subtitle file found next to a video.
type File struct {
	Name   string `json:"name"`   // file name, in the video's folder
	Format string `json:"format"` // "srt", "ass" or "vtt"
	Lang   string `json:"lang"`   // from the name: "zh-Hans", "zh-Hant", "ja", "en", … or ""
	Tag    string `json:"tag"`    // what the name says between the video's name and the extension
}

// Beside lists the subtitle files for video in its folder: those whose
// name is the video's, optionally followed by a tag, then a subtitle
// extension ("ep.srt", "ep.chs.ass", "ep[CHT].srt", "ep.ja.vtt").
func Beside(video string) []File {
	dir, base := filepath.Split(video)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []File
	for _, e := range ents {
		name := e.Name()
		f, ok := Formats[strings.ToLower(filepath.Ext(name))]
		if !ok || e.IsDir() || len(name) < len(stem) || !strings.EqualFold(name[:len(stem)], stem) {
			continue
		}
		tag := strings.TrimSuffix(name[len(stem):], filepath.Ext(name))
		// "ep 2.srt" belongs to "ep 2", not "ep": a tag must be set apart.
		if tag != "" && !strings.ContainsAny(tag[:1], ".-_ [(（【") {
			continue
		}
		out = append(out, File{Name: name, Format: f, Tag: strings.Trim(tag, ".-_ []()（）【】"), Lang: LangOfTag(tag)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

var tagWord = regexp.MustCompile(`[A-Za-z][A-Za-z0-9]*(?:-[A-Za-z]+)?|简|繁|中|日|英`)

// LangOfTag reads a language from a file name tag or a Matroska language
// code: the first word that names one.
func LangOfTag(tag string) string {
	for _, w := range tagWord.FindAllString(tag, -1) {
		switch strings.ToLower(w) {
		case "chs", "sc", "gb", "zh-hans", "zh-cn", "hans", "简", "zh", "chi", "zho", "chinese", "中":
			return "zh-Hans"
		case "cht", "tc", "big5", "zh-hant", "zh-tw", "zh-hk", "hant", "繁":
			return "zh-Hant"
		case "ja", "jp", "jpn", "japanese", "日":
			return "ja"
		case "en", "eng", "english", "英":
			return "en"
		case "ko", "kor", "korean":
			return "ko"
		}
	}
	return ""
}
