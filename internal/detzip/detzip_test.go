package detzip

import (
	"archive/zip"
	"bytes"
	"testing"
)

// An archive that declares a huge entry is refused before anything is
// inflated (a zip bomb), and one within the limits still opens.
func TestOpenLimits(t *testing.T) {
	mk := func(size uint64) []byte {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, err := zw.CreateRaw(&zip.FileHeader{Name: "a.json", Method: zip.Store, CompressedSize64: 2, UncompressedSize64: size})
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte("{}"))
		zw.Close()
		return buf.Bytes()
	}
	if _, err := Open(mk(MaxEntrySize + 1)); err == nil {
		t.Fatal("an entry declaring more than the limit was accepted")
	}
	if _, err := Open(mk(2)); err != nil {
		t.Fatalf("a small archive was refused: %v", err)
	}
}
