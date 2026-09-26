// Package safeswap replaces a file with a new version so that no crash or
// power cut can lose both: the new file is flushed to disk first, the
// original is moved aside, the new one moved in, and only then is the
// original removed. Recover finishes or undoes a swap that was cut short;
// the library runs it when it finds the leftovers, and so does the next
// swap of the same file.
package safeswap

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OldSuffix marks the original while it is set aside.
const OldSuffix = ".jusplay-old"

// Temp is where a new version of path is written before the swap: a dot
// file next to it (same volume, so the rename is atomic; hidden from the
// library's scan).
func Temp(path string) string {
	dir, base := filepath.Split(path)
	return filepath.Join(dir, "."+strings.TrimSuffix(base, filepath.Ext(base))+".jusplay"+filepath.Ext(base))
}

// Sync flushes a written file to disk.
func Sync(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Swap replaces path with next (a finished file on the same volume). On
// failure path is left as it was, or restored.
func Swap(next, path string) error {
	old := path + OldSuffix
	if exists(old) {
		return fmt.Errorf("%s exists; run Recover first", old)
	}
	if err := Sync(next); err != nil {
		return err
	}
	if err := renameRetry(path, old); err != nil {
		return err
	}
	if err := os.Rename(next, path); err != nil {
		if rerr := os.Rename(old, path); rerr != nil {
			return fmt.Errorf("%v; restoring the original also failed, it is at %s", err, old)
		}
		return err
	}
	if err := os.Remove(old); err != nil {
		return fmt.Errorf("replaced, but could not remove the old copy %s: %w", old, err)
	}
	return nil
}

// Recover looks for what an interrupted Swap of path left behind and puts
// things right. It returns what it did ("" when there was nothing to do).
//
//   - original aside, nothing at path: the swap stopped between its two
//     renames; the original goes back (the new file, if any, is dropped).
//   - original aside and a file at path: only the removal was missing; the
//     file at path is the new version, which was complete before the swap
//     began, so the old copy is removed.
//   - a temporary file with the original in place: an unfinished write,
//     removed.
func Recover(path string) (string, error) {
	old, tmp := path+OldSuffix, Temp(path)
	var did []string
	switch {
	case exists(old) && !exists(path):
		if err := os.Rename(old, path); err != nil {
			return "", fmt.Errorf("restoring %s: %w", path, err)
		}
		did = append(did, "restored the original")
	case exists(old):
		if err := os.Remove(old); err != nil {
			return "", err
		}
		did = append(did, "removed the old copy")
	}
	if exists(path) {
		for _, p := range []string{tmp, tmp + ".partial"} {
			if exists(p) {
				if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
					return strings.Join(did, "; "), err
				}
				did = append(did, "removed an unfinished "+filepath.Base(p))
			}
		}
	}
	return strings.Join(did, "; "), nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// renameRetry retries briefly: on Windows a file still open by a process
// that is being stopped (a cancelled stream) cannot be renamed.
func renameRetry(from, to string) error {
	var err error
	for i := 0; i < 20; i++ {
		if err = os.Rename(from, to); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return err
}
