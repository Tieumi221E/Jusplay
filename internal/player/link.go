package player

// Links to a moment of a video (Jus contract 3):
//
//	jus://play/<folder id>/<path in the folder>?t=<seconds>
//	jus://play/file/<absolute path>?t=<seconds>      a file outside every folder
//
// The folder is named by its own id (library.FolderID), so a link written
// into a note still opens the video when the folder has moved. Every
// capability that takes a video takes a link too; player open starts at
// the link's moment.

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Tieumi221E/Jus/capreg"
	"github.com/Tieumi221E/Jusplay/internal/library"
)

const linkPrefix = "jus://play/"

// LinkOf is the link to e at t seconds (t <= 0: no moment).
func (s *Server) LinkOf(e library.Entry, t float64) string {
	var path string
	if id := s.lib.FolderID(e.Folder); e.Folder != "" && id != "" {
		rel, err := filepath.Rel(e.Folder, e.Path)
		if err == nil {
			path = id + "/" + escapePath(filepath.ToSlash(rel))
		}
	}
	if path == "" {
		path = "file/" + escapePath(filepath.ToSlash(e.Path))
	}
	link := linkPrefix + path
	if t > 0 {
		link += "?t=" + strconv.FormatFloat(float64(int64(t*1000))/1000, 'f', -1, 64)
	}
	return link
}

func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, x := range parts {
		parts[i] = url.PathEscape(x)
	}
	return strings.Join(parts, "/")
}

// resolveLink is the video and moment a link names.
func (s *Server) resolveLink(link string) (library.Entry, float64, error) {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "jus" || u.Host != "play" {
		return library.Entry{}, 0, capreg.Usagef("not a jus://play link: %q", link)
	}
	var t float64
	if v := u.Query().Get("t"); v != "" {
		if t, err = strconv.ParseFloat(v, 64); err != nil || t < 0 {
			return library.Entry{}, 0, capreg.Usagef("bad moment t=%q in %s", v, link)
		}
	}
	rest := strings.TrimPrefix(u.Path, "/")
	first, rel, ok := strings.Cut(rest, "/")
	if !ok || rel == "" {
		return library.Entry{}, 0, capreg.Usagef("a jus://play link names a folder and a file: %q", link)
	}
	var path string
	if first == "file" {
		path = filepath.FromSlash(rel)
	} else {
		root, ok := s.lib.FolderByID(first)
		if !ok {
			return library.Entry{}, 0, capreg.NotFoundf("the folder of %s is not in the library (library add <folder>)", link)
		}
		path = filepath.Join(root, filepath.FromSlash(rel))
	}
	e, err := s.resolve(path)
	return e, t, err
}

// linkMoment is the moment a video reference names, when it is a link.
func linkMoment(ref string) (float64, bool) {
	if !strings.HasPrefix(ref, linkPrefix) {
		return 0, false
	}
	u, err := url.Parse(ref)
	if err != nil {
		return 0, false
	}
	t, err := strconv.ParseFloat(u.Query().Get("t"), 64)
	return t, err == nil
}

// linkTitle is how a link reads in a note: the video's title and the moment.
func linkTitle(e library.Entry, t float64) string {
	title := e.Title
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(e.Path), filepath.Ext(e.Path))
	}
	if t > 0 {
		title += " " + clock(t)
	}
	return title
}

// markdownLink is [title](link), with the brackets in the title escaped.
func markdownLink(title, link string) string {
	r := strings.NewReplacer(`\`, `\\`, "[", `\[`, "]", `\]`)
	return fmt.Sprintf("[%s](%s)", r.Replace(title), link)
}
