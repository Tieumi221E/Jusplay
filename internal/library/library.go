// Package library keeps the local media library: folders the user added,
// the videos found in them, facts probed from each file, and playback
// progress. The records sit in each folder (see Library); apart from
// progress they can always be rebuilt from the files.
package library

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Tieumi221E/Jusplay/internal/mkv"
	"github.com/Tieumi221E/Jusplay/internal/safeswap"
)

// Extensions the scanner picks up.
var videoExt = map[string]bool{".mkv": true, ".mp4": true, ".m4v": true, ".webm": true}

type Probe struct {
	Duration   float64 `json:"duration"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	VideoCodec string  `json:"videoCodec"`
	Profile    string  `json:"profile,omitempty"`
	AudioCodec string  `json:"audioCodec,omitempty"`
	// Attached is true when the file carries jusplay attachments.
	Attached bool   `json:"attached"`
	Error    string `json:"error,omitempty"`
}

type Entry struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Folder string `json:"folder"` // the added folder it was found under ("" if opened directly)
	Series string `json:"series"`
	// Season is set when the name says so ("3期", "S2", "2nd Season");
	// several seasons often share one folder.
	Season  *int     `json:"season"`
	Episode *float64 `json:"episode"`
	Title   string   `json:"title"`
	// Subtitle is what the file name says after the episode number
	// ("Show - 01 - 旅立ち" -> "旅立ち"), if anything.
	Subtitle string    `json:"subtitle,omitempty"`
	Size     int64     `json:"size"`
	ModTime  time.Time `json:"modTime"`
	Probe    *Probe    `json:"probe"`
	// Comments is a snapshot directory or raw ZIP linked by the user, used
	// when the file has no attachments.
	Comments string `json:"comments,omitempty"`
	// CommentCount is filled the first time comments are loaded.
	CommentCount *int `json:"commentCount,omitempty"`
	// Playback progress.
	Position   float64    `json:"position"`
	Watched    bool       `json:"watched"`
	LastPlayed *time.Time `json:"lastPlayed,omitempty"`
	Missing    bool       `json:"missing"` // file gone at the last scan
	// OffsetMs is the comment offset chosen for this file (nil: the one
	// its comment data records).
	OffsetMs *int64 `json:"offsetMs,omitempty"`
	// SubPick and SubPick2 are the subtitle tracks chosen for this file
	// (primary, secondary): "file:<name>" beside it, "manual", "mkv:<n>",
	// "ai:src", "ai:tr"; "off" for none; "" not chosen yet. SubFile is a
	// subtitle file picked by hand ("manual").
	SubPick  string `json:"subPick,omitempty"`
	SubPick2 string `json:"subPick2,omitempty"`
	SubFile  string `json:"subFile,omitempty"`
}

// Library is the media folders and what is known about the files in them.
//
// Like a notes vault, each media folder keeps its own records in a hidden
// ".jusplay" folder inside it: the files found, what was probed, progress
// and the per-video comment offset, with paths relative to the folder, so
// the records move with it (another drive letter, another computer), and
// adding a folder that has them brings them back. The app itself keeps only
// the list of folders (foldersPath). Files opened on their own, and folders
// that cannot be written (a read-only share), are kept in memory only:
// nothing about them is written anywhere.
type Library struct {
	foldersPath string
	mu          sync.Mutex
	folders     []string
	vaults      map[string]*vault // by lower-case folder path
	entries     map[string]*Entry // every entry, by id

	progressDone, progressTotal atomic.Int64
}

// VaultDir is the name of the records folder inside each media folder.
const VaultDir = ".jusplay"

type vault struct {
	root string
	// err is why the records cannot be saved ("" when they can).
	err string
}

func (v *vault) dir() string  { return filepath.Join(v.root, VaultDir) }
func (v *vault) file() string { return filepath.Join(v.dir(), "library.json") }

// vaultDoc is a media folder's library.json. Entries carry paths relative
// to the folder; their id and folder are derived when it is loaded.
type vaultDoc struct {
	Version int            `json:"version"`
	Entries []*storedEntry `json:"entries"`
}

type storedEntry struct {
	*Entry
	ID     string `json:"id,omitempty"`
	Folder string `json:"folder,omitempty"`
	Path   string `json:"path"`
}

type foldersDoc struct {
	Version int      `json:"version"`
	Folders []string `json:"folders"`
}

const scanWorkers = 4

// Progress reports files probed so far and the total of the running scan.
func (l *Library) Progress() (done, total int64) {
	return l.progressDone.Load(), l.progressTotal.Load()
}

// Open loads the folder list at foldersPath ("" for none: everything in
// memory) and the records of each folder. A missing list starts empty; an
// unreadable one is an error, so it is never silently replaced. A folder
// whose records cannot be read (drive unplugged) starts empty.
func Open(foldersPath string) (*Library, error) {
	l := &Library{foldersPath: foldersPath, vaults: map[string]*vault{}, entries: map[string]*Entry{}}
	if foldersPath == "" {
		return l, nil
	}
	b, err := os.ReadFile(foldersPath)
	if errors.Is(err, fs.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return nil, err
	}
	var d foldersDoc
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("%s: %w", foldersPath, err)
	}
	for _, f := range d.Folders {
		// Only absolute paths: an empty or relative one would mean the
		// current directory, which is nobody's media folder.
		if filepath.IsAbs(f) {
			l.addVault(f)
		}
	}
	return l, nil
}

// addVault adds folder dir and loads its records; callers hold mu (or own l).
func (l *Library) addVault(dir string) {
	dir = filepath.Clean(dir)
	key := strings.ToLower(dir)
	if l.vaults[key] != nil {
		return
	}
	v := &vault{root: dir}
	l.vaults[key] = v
	l.folders = append(l.folders, dir)
	b, err := os.ReadFile(v.file())
	if err != nil {
		return // none yet, or the drive is away
	}
	var d vaultDoc
	if err := json.Unmarshal(b, &d); err != nil {
		// Left as it is; this folder's records stay in memory until it is fixed.
		v.err = fmt.Sprintf("%s: %v", v.file(), err)
		return
	}
	for _, se := range d.Entries {
		if se == nil || se.Entry == nil || se.Path == "" || filepath.IsAbs(se.Path) || strings.HasPrefix(filepath.Clean(se.Path), "..") {
			continue
		}
		e := se.Entry
		e.Path = filepath.Join(dir, se.Path)
		e.ID, e.Folder = IDFor(e.Path), dir
		l.entries[e.ID] = e
	}
}

// saveVault writes the records of one folder; callers hold mu. A folder
// that cannot be written keeps them in memory and says why (FolderInfos);
// that is not an error of the caller's change.
func (l *Library) saveVault(v *vault) {
	if l.foldersPath == "" || strings.HasPrefix(v.err, v.file()+":") {
		return // in memory, or a records file we could not read: never overwritten
	}
	d := vaultDoc{Version: 2, Entries: []*storedEntry{}}
	for _, e := range l.entries {
		if e.Folder != v.root {
			continue
		}
		rel, err := filepath.Rel(v.root, e.Path)
		if err != nil {
			continue
		}
		d.Entries = append(d.Entries, &storedEntry{Entry: e, Path: rel})
	}
	sort.Slice(d.Entries, func(i, j int) bool { return d.Entries[i].Path < d.Entries[j].Path })
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		v.err = err.Error()
		return
	}
	if err := writeAtomic(v.file(), b); err != nil {
		v.err = err.Error()
		return
	}
	v.err = ""
	hide(v.dir())
}

// hide marks a records folder hidden (appdir.Hide, set by SetHide).
var hide = func(string) {}

// SetHide installs the function that marks a records folder hidden.
func SetHide(f func(string)) { hide = f }

func (l *Library) saveFolders() error {
	if l.foldersPath == "" {
		return nil
	}
	b, err := json.MarshalIndent(foldersDoc{Version: 2, Folders: l.folders}, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(l.foldersPath, b)
}

// saveOf saves the records of the folder e belongs to, if any; callers hold mu.
func (l *Library) saveOf(e *Entry) {
	if v := l.vaults[strings.ToLower(e.Folder)]; e.Folder != "" && v != nil {
		l.saveVault(v)
	}
}

func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".partial"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// IDFor is a stable id for a path (case-insensitive on Windows).
func IDFor(path string) string {
	h := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(path))))
	return hex.EncodeToString(h[:8])
}

func (l *Library) Folders() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.folders...)
}

// FolderInfo is an added folder and whether its records are saved.
type FolderInfo struct {
	Path string `json:"path"`
	// Error is why its records stay in memory ("" when they are saved).
	Error string `json:"error,omitempty"`
}

func (l *Library) FolderInfos() []FolderInfo {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]FolderInfo, 0, len(l.folders))
	for _, f := range l.folders {
		out = append(out, FolderInfo{Path: f, Error: l.vaults[strings.ToLower(f)].err})
	}
	return out
}

// DataDir is where e's cached thumbnail and index go: its folder's records
// folder, or "" (outside any folder, or one that cannot be written), for
// the caller's temporary place.
func (l *Library) DataDir(e Entry) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if v := l.vaults[strings.ToLower(e.Folder)]; e.Folder != "" && v != nil && v.err == "" && l.foldersPath != "" {
		return v.dir()
	}
	return ""
}

// AddFolder adds a media folder; records already in it are loaded.
func (l *Library) AddFolder(dir string) error {
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("%q is not a full path", dir)
	}
	dir = filepath.Clean(dir)
	st, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return fmt.Errorf("%s is not a folder", dir)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.vaults[strings.ToLower(dir)] != nil {
		return nil
	}
	l.addVault(dir)
	return l.saveFolders()
}

// RemoveFolder forgets a folder: its entries leave the library, and its
// records stay in it, so adding it again brings them back. Files are
// untouched.
func (l *Library) RemoveFolder(dir string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := strings.ToLower(filepath.Clean(dir))
	v := l.vaults[key]
	if v == nil {
		return nil
	}
	delete(l.vaults, key)
	out := l.folders[:0]
	for _, f := range l.folders {
		if strings.ToLower(f) != key {
			out = append(out, f)
		}
	}
	l.folders = out
	for id, e := range l.entries {
		if e.Folder == v.root {
			delete(l.entries, id)
		}
	}
	return l.saveFolders()
}

// Entries returns copies of all entries.
func (l *Library) Entries() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Entry, 0, len(l.entries))
	for _, e := range l.entries {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return Less(out[i], out[j]) })
	return out
}

// Less orders by series, season (unset = 1), episode number, then file
// name; entries without an episode number (OVA, films) come last in their
// season.
func Less(a, b Entry) bool {
	if a.Series != b.Series {
		return a.Series < b.Series
	}
	if sa, sb := seasonOf(a), seasonOf(b); sa != sb {
		return sa < sb
	}
	switch {
	case a.Episode != nil && b.Episode != nil && *a.Episode != *b.Episode:
		return *a.Episode < *b.Episode
	case (a.Episode == nil) != (b.Episode == nil):
		return a.Episode != nil
	}
	return strings.ToLower(filepath.Base(a.Path)) < strings.ToLower(filepath.Base(b.Path))
}

func (l *Library) Get(id string) (Entry, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[id]
	if !ok {
		return Entry{}, false
	}
	return *e, true
}

// Update changes one entry under the lock and saves its folder's records.
func (l *Library) Update(id string, f func(*Entry)) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[id]
	if !ok {
		return fmt.Errorf("no entry %s", id)
	}
	f(e)
	l.saveOf(e)
	return nil
}

// Ensure adds a file opened directly. Under an added folder it joins that
// folder (and its records); anywhere else it is kept in memory only.
func (l *Library) Ensure(path string) (Entry, error) {
	path = filepath.Clean(path)
	id := IDFor(path)
	l.mu.Lock()
	e, ok := l.entries[id]
	folder := ""
	for _, f := range l.folders {
		if rel, err := filepath.Rel(f, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
			folder = f
			break
		}
	}
	l.mu.Unlock()
	if ok {
		return *e, nil
	}
	st, err := os.Stat(path)
	if err != nil {
		return Entry{}, err
	}
	ne := newEntry(path, folder, st)
	ne.Probe = probe(path)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries[id] = ne
	l.saveOf(ne)
	return *ne, nil
}

// Import adds folders and entries from another library (the single file
// of earlier versions): each folder is added, and each of its entries is
// taken; for a file the folder's records already know, only its progress,
// and only if it was played later there. Entries outside the folders
// (files opened on their own) are left out: they are not kept.
func (l *Library) Import(folders []string, entries []Entry) (added int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, f := range folders {
		if st, e := os.Stat(f); e != nil || !st.IsDir() {
			continue
		}
		l.addVault(f)
	}
	touched := map[*vault]bool{}
	for _, e := range entries {
		v := l.vaults[strings.ToLower(filepath.Clean(e.Folder))]
		if e.Folder == "" || v == nil {
			continue
		}
		e.Folder = v.root
		e.ID = IDFor(e.Path)
		if cur, ok := l.entries[e.ID]; ok {
			if e.LastPlayed != nil && (cur.LastPlayed == nil || e.LastPlayed.After(*cur.LastPlayed)) {
				cur.Position, cur.Watched, cur.LastPlayed = e.Position, e.Watched, e.LastPlayed
				if cur.OffsetMs == nil {
					cur.OffsetMs = e.OffsetMs
				}
				touched[v] = true
				added++
			}
			continue
		}
		ne := e
		l.entries[e.ID] = &ne
		touched[v] = true
		added++
	}
	for v := range touched {
		l.saveVault(v)
	}
	return added, l.saveFolders()
}

// ScanStats reports what a scan did.
type ScanStats struct {
	Folders, Files, Added, Changed, Missing int
	Took                                    time.Duration
	// FolderErrors lists added folders that could not be read (unplugged
	// drive, renamed folder); their files are not marked missing.
	FolderErrors []string
	// Recovered lists files whose interrupted replacement (an embed cut
	// short by a crash or power cut) was put right, and how.
	Recovered []string
}

// Scan walks every folder, adds new files, re-probes changed ones (size or
// modification time differ) and marks vanished ones missing. Progress is
// kept for files that are still there.
func (l *Library) Scan() (ScanStats, error) {
	start := time.Now()
	st := ScanStats{}
	seen := map[string]bool{}
	type found struct {
		path, folder string
		info         fs.FileInfo
	}
	var files []found
	unreadable := map[string]bool{}
	for _, dir := range l.Folders() {
		st.Folders++
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			if err == nil {
				err = fmt.Errorf("not a folder")
			}
			st.FolderErrors = append(st.FolderErrors, fmt.Sprintf("%s: %v", dir, err))
			unreadable[strings.ToLower(dir)] = true
			continue
		}
		filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // unreadable subfolder: skip, keep scanning
			}
			name := d.Name()
			// An interrupted replacement (safeswap): finish or undo it. The
			// original sorts before its leftover, so when it had to be
			// restored it was not seen yet; take it now.
			if !d.IsDir() && strings.HasSuffix(name, safeswap.OldSuffix) {
				orig := strings.TrimSuffix(p, safeswap.OldSuffix)
				_, missing := os.Stat(orig)
				did, err := safeswap.Recover(orig)
				if err != nil {
					st.FolderErrors = append(st.FolderErrors, fmt.Sprintf("%s: %v", orig, err))
				} else if did != "" {
					st.Recovered = append(st.Recovered, orig+": "+did)
				}
				if info, err := os.Stat(orig); missing != nil && err == nil && videoExt[strings.ToLower(filepath.Ext(orig))] {
					files = append(files, found{orig, dir, info})
				}
				return nil
			}
			if d.IsDir() {
				if p != dir && (strings.HasPrefix(name, ".") || strings.EqualFold(name, "$RECYCLE.BIN")) {
					return filepath.SkipDir
				}
				return nil
			}
			// Dot-files are ours or the system's (an embed's temporary copy).
			if !videoExt[strings.ToLower(filepath.Ext(name))] || strings.HasSuffix(name, ".partial") || strings.HasPrefix(name, ".") {
				return nil
			}
			info, err := d.Info()
			if err == nil {
				files = append(files, found{p, dir, info})
			}
			return nil
		})
	}
	l.progressTotal.Store(int64(len(files)))
	l.progressDone.Store(0)
	// Probing is the slow part (reading each new or changed file's headers); run a
	// few at once. Everything else happens under mu.
	work := make(chan found)
	var wg sync.WaitGroup
	for w := 0; w < scanWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range work {
				id := IDFor(f.path)
				l.mu.Lock()
				old := l.entries[id]
				unchanged := old != nil && old.Size == f.info.Size() && old.ModTime.Equal(f.info.ModTime()) && old.Probe != nil
				if unchanged {
					// Names are re-read every scan (cheap, and parsing improves);
					// only the probe is cached.
					n := ParseName(f.path, f.folder)
					old.Missing, old.Folder = false, f.folder
					old.Series, old.Season, old.Episode, old.Title, old.Subtitle = n.Series, n.Season, n.Episode, n.Title, n.Subtitle
				}
				l.mu.Unlock()
				if !unchanged {
					ne := newEntry(f.path, f.folder, f.info)
					ne.Probe = probe(f.path)
					l.mu.Lock()
					if old != nil {
						st.Changed++
						ne.Position, ne.Watched, ne.LastPlayed, ne.Comments, ne.OffsetMs, ne.CommentCount =
							old.Position, old.Watched, old.LastPlayed, old.Comments, old.OffsetMs, old.CommentCount
						ne.SubPick, ne.SubPick2, ne.SubFile = old.SubPick, old.SubPick2, old.SubFile
					} else {
						st.Added++
					}
					// A folder removed while the scan ran does not come back.
					if l.vaults[strings.ToLower(f.folder)] != nil {
						l.entries[id] = ne
					}
					l.mu.Unlock()
				}
				l.progressDone.Add(1)
			}
		}()
	}
	for _, f := range files {
		st.Files++
		seen[IDFor(f.path)] = true
		work <- f
	}
	close(work)
	wg.Wait()
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.entries {
		if e.Folder != "" && !seen[e.ID] && !e.Missing && !unreadable[strings.ToLower(e.Folder)] {
			e.Missing = true
			st.Missing++
		}
	}
	st.Took = time.Since(start)
	for _, v := range l.vaults {
		if !unreadable[strings.ToLower(v.root)] {
			l.saveVault(v)
		}
	}
	return st, nil
}

func seasonOf(e Entry) int {
	if e.Season == nil {
		return 1
	}
	return *e.Season
}

func newEntry(path, folder string, info fs.FileInfo) *Entry {
	n := ParseName(path, folder)
	return &Entry{ID: IDFor(path), Path: path, Folder: folder, Series: n.Series, Season: n.Season, Episode: n.Episode, Title: n.Title, Subtitle: n.Subtitle,
		Size: info.Size(), ModTime: info.ModTime()}
}

func probe(path string) *Probe {
	p, err := mkv.ProbeFile(path)
	if err != nil {
		return &Probe{Error: err.Error()}
	}
	out := &Probe{Duration: p.Duration}
	for _, s := range p.Media() {
		switch {
		case s.CodecType == "video" && out.VideoCodec == "":
			out.VideoCodec, out.Profile = s.CodecName, s.Profile
			out.Width, out.Height = s.Width, s.Height
		case s.CodecType == "audio" && out.AudioCodec == "":
			out.AudioCodec = s.CodecName
		}
	}
	out.Attached = len(p.Ours()) > 0
	return out
}

var (
	folderPrefix = regexp.MustCompile(`^-[A-Za-z0-9]{1,3}-\s*`)
	// genericFolder names say what a folder holds, not which show: a tool's
	// output folder, a season or disc of a show. The series is then the
	// folder above.
	genericFolder = regexp.MustCompile(`(?i)^(?:anime_processed|processed|output|out|encoded?|remux(?:ed)?|videos?|downloads?|` +
		`season\s*\d{1,2}|s\d{1,2}|第?\d{1,2}期|disc\s*\d{1,2}|dis[ck]\d{1,2}|bd|dvd|bdrip|web-?dl)$`)
	bracketed = regexp.MustCompile(`[\[【(（][^\]】)）]*[\]】)）]`)
	episodeRe = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\s-\s*(?:ep?\.?\s*)?(\d{1,4}(?:[._]\d)?)(?:v\d)?(?:\s|$)`),
		regexp.MustCompile(`(?i)(?:第|#|\bep?\.?\s*)(\d{1,4}(?:[._]\d)?)\s*(?:話|话|集)`),
		regexp.MustCompile(`(?:^|\s)(\d{1,3})(?:\s|$)`),
	}
	seasonRe = []*regexp.Regexp{
		regexp.MustCompile(`第?(\d{1,2})期`),
		regexp.MustCompile(`(?i)(\d{1,2})(?:st|nd|rd|th)\s*season`),
		regexp.MustCompile(`(?i)season\s*(\d{1,2})`),
		regexp.MustCompile(`(?i)(?:^|[\s_])s(\d{1,2})(?:e\d+)?(?:$|[\s_])`),
	}
)

// Name is what ParseName reads from a path.
type Name struct {
	Series   string
	Season   *int
	Episode  *float64
	Title    string
	Subtitle string
}

// ParseName derives series, episode number and a display title from a
// path. The series is the parent folder (without markers like "-A- "),
// or the one above when the parent is a generic one ("anime_processed",
// "Season 2"; genericFolder), unless that is the added root folder (or
// the file was opened on its own): then it is the file name before the
// episode number.
func ParseName(path, root string) Name {
	var series, title string
	var episode *float64
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	clean := strings.TrimSpace(bracketed.ReplaceAllString(base, " "))
	clean = strings.Join(strings.Fields(clean), " ")
	at, after := -1, -1
	for _, re := range episodeRe {
		if m := re.FindStringSubmatchIndex(clean); m != nil {
			if v, err := strconv.ParseFloat(strings.ReplaceAll(clean[m[2]:m[3]], "_", "."), 64); err == nil {
				episode = &v
				at, after = m[0], m[1]
				break
			}
		}
	}
	var subtitle string
	if after > 0 {
		subtitle = strings.Trim(clean[after:], " -_~・")
	}
	// The season is named before the episode number (or anywhere if none).
	head := clean
	if at > 0 {
		head = clean[:at]
	}
	var season *int
	for _, re := range seasonRe {
		if m := re.FindStringSubmatch(head); m != nil {
			if v, err := strconv.Atoi(m[1]); err == nil && v > 0 {
				season = &v
				break
			}
		}
	}
	dir := filepath.Dir(path)
	atRoot := func(d string) bool { return root == "" || strings.EqualFold(filepath.Clean(d), filepath.Clean(root)) }
	if !atRoot(dir) && genericFolder.MatchString(strings.TrimSpace(filepath.Base(dir))) {
		if season == nil { // "Season 2", "S02", "第2期"
			for _, re := range seasonRe {
				if m := re.FindStringSubmatch(" " + filepath.Base(dir) + " "); m != nil {
					if v, err := strconv.Atoi(m[1]); err == nil && v > 0 {
						season = &v
						break
					}
				}
			}
		}
		dir = filepath.Dir(dir)
	}
	if atRoot(dir) {
		series = clean
		if at > 0 {
			series = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(clean[:at]), "-_ "))
		}
	} else {
		series = folderPrefix.ReplaceAllString(filepath.Base(dir), "")
	}
	title = clean
	if title == "" {
		title = base
	}
	return Name{Series: series, Season: season, Episode: episode, Title: title, Subtitle: subtitle}
}
