package media

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The index cache keeps a Matroska file's sample tables (every sample's
// position, size, decode and presentation time, duration and key flag, per
// track), so opening a video again reads only its headers: the index walks
// the whole file (47 ms warm for a 24-minute episode, much longer from a
// cold disk). A file is keyed by its path, size and modification time, so a
// replaced or re-muxed file is indexed afresh. MP4 files carry their sample
// tables in the header and are not cached.
//
// Format: "JPIX2\n", uvarint track count, then per track: uvarint track
// number, uvarint sample count, and per sample: varint decode time delta,
// varint presentation - decode time, varint position delta, uvarint size,
// uvarint duration<<1|key.
const cacheMagic = "JPIX2\n"

// CacheFile is where the index of path lives in dir ("" when path cannot be stat'ed).
func CacheFile(dir, path string) string {
	fi, err := os.Stat(path)
	if err != nil || dir == "" {
		return ""
	}
	abs, _ := filepath.Abs(path)
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d|%s", strings.ToLower(abs), fi.Size(), fi.ModTime().UnixNano(), cacheMagic)))
	return filepath.Join(dir, hex.EncodeToString(h[:12])+".idx")
}

type index struct {
	tracks map[uint64][]sample
}

func encodeIndex(x *index) []byte {
	b := []byte(cacheMagic)
	b = binary.AppendUvarint(b, uint64(len(x.tracks)))
	for n, ss := range x.tracks {
		b = binary.AppendUvarint(b, n)
		b = binary.AppendUvarint(b, uint64(len(ss)))
		var dts, pos int64
		for _, s := range ss {
			b = binary.AppendVarint(b, s.dts-dts)
			b = binary.AppendVarint(b, s.pts-s.dts)
			b = binary.AppendVarint(b, s.pos-pos)
			b = binary.AppendUvarint(b, uint64(s.size))
			k := uint64(0)
			if s.key {
				k = 1
			}
			b = binary.AppendUvarint(b, uint64(s.dur)<<1|k)
			dts, pos = s.dts, s.pos
		}
	}
	return b
}

var errBadCache = errors.New("bad index cache")

func decodeIndex(b []byte) (*index, error) {
	if !bytes.HasPrefix(b, []byte(cacheMagic)) {
		return nil, errBadCache
	}
	r := bytes.NewReader(b[len(cacheMagic):])
	nt, err := binary.ReadUvarint(r)
	if err != nil || nt > 64 {
		return nil, errBadCache
	}
	x := &index{tracks: map[uint64][]sample{}}
	for i := uint64(0); i < nt; i++ {
		num, err1 := binary.ReadUvarint(r)
		cnt, err2 := binary.ReadUvarint(r)
		// Each sample takes at least five bytes.
		if err1 != nil || err2 != nil || cnt > uint64(r.Len())/5 {
			return nil, errBadCache
		}
		ss := make([]sample, cnt)
		var dts, pos int64
		for j := range ss {
			dd, e1 := binary.ReadVarint(r)
			cto, e2 := binary.ReadVarint(r)
			dp, e3 := binary.ReadVarint(r)
			size, e4 := binary.ReadUvarint(r)
			dk, e5 := binary.ReadUvarint(r)
			if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || size > 1<<31 {
				return nil, errBadCache
			}
			dts += dd
			pos += dp
			ss[j] = sample{pos: pos, size: uint32(size), dts: dts, pts: dts + cto, dur: uint32(dk >> 1), key: dk&1 == 1}
		}
		x.tracks[num] = ss
	}
	if r.Len() != 0 {
		return nil, errBadCache
	}
	return x, nil
}

func readCache(path string) (*index, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decodeIndex(b)
}

func writeCache(path string, x *index) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".partial"
	if err := os.WriteFile(tmp, encodeIndex(x), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
