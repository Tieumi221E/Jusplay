// Package ebml reads and writes EBML, the binary format of Matroska and
// WebM: elements of (ID, size, data), sizes as variable-length integers,
// masters holding other elements. Only what Jusplay needs, written for it.
package ebml

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// UnknownSize marks a master whose size was not written (live streams):
// it ends where an element that cannot be its child begins.
const UnknownSize = -1

// Element is an element header read from a file.
type Element struct {
	ID      uint32
	Start   int64 // offset of the ID
	DataPos int64 // offset of the data
	Size    int64 // data size, or UnknownSize
}

// End is the offset just past the element (DataPos for unknown sizes).
func (e Element) End() int64 {
	if e.Size == UnknownSize {
		return e.DataPos
	}
	return e.DataPos + e.Size
}

// HeaderLen is the size of the ID and size fields.
func (e Element) HeaderLen() int64 { return e.DataPos - e.Start }

var ErrInvalid = errors.New("invalid EBML")

// ReadHeader reads the element header at off.
func ReadHeader(r io.ReaderAt, off int64) (Element, error) {
	var b [12]byte
	n, err := r.ReadAt(b[:], off)
	if n == 0 {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return Element{}, err
	}
	return ParseHeader(b[:n], off)
}

// ParseHeader parses an element header from b, which starts at file offset off.
func ParseHeader(b []byte, off int64) (Element, error) {
	id, il, err := readID(b)
	if err != nil {
		return Element{}, fmt.Errorf("%w at %d: %v", ErrInvalid, off, err)
	}
	size, sl, unknown, err := readVint(b[il:])
	if err != nil {
		return Element{}, fmt.Errorf("%w at %d: %v", ErrInvalid, off, err)
	}
	e := Element{ID: id, Start: off, DataPos: off + int64(il+sl), Size: int64(size)}
	if unknown {
		e.Size = UnknownSize
	}
	return e, nil
}

// readID reads an element ID (the length marker kept, as IDs are written).
func readID(b []byte) (uint32, int, error) {
	if len(b) == 0 {
		return 0, 0, io.ErrUnexpectedEOF
	}
	n := lenOf(b[0])
	if n == 0 || n > 4 {
		return 0, 0, errors.New("bad ID")
	}
	if len(b) < n {
		return 0, 0, io.ErrUnexpectedEOF
	}
	var id uint32
	for i := 0; i < n; i++ {
		id = id<<8 | uint32(b[i])
	}
	return id, n, nil
}

func lenOf(first byte) int {
	for i := 0; i < 8; i++ {
		if first&(0x80>>i) != 0 {
			return i + 1
		}
	}
	return 0
}

// readVint reads a size (the length marker removed). All ones means unknown.
func readVint(b []byte) (v uint64, n int, unknown bool, err error) {
	if len(b) == 0 {
		return 0, 0, false, io.ErrUnexpectedEOF
	}
	n = lenOf(b[0])
	if n == 0 {
		return 0, 0, false, errors.New("bad size")
	}
	if len(b) < n {
		return 0, 0, false, io.ErrUnexpectedEOF
	}
	v = uint64(b[0]) & (0xff >> n)
	all := v == uint64(0xff>>n)
	for i := 1; i < n; i++ {
		v = v<<8 | uint64(b[i])
		all = all && b[i] == 0xff
	}
	return v, n, all, nil
}

// Vint reads a variable-length integer from the start of b (as in block
// headers: the track number). It returns the value and its length.
func Vint(b []byte) (uint64, int, error) {
	v, n, _, err := readVint(b)
	return v, n, err
}

// SignedVint reads the signed form used by EBML lacing (value - bias).
func SignedVint(b []byte) (int64, int, error) {
	v, n, _, err := readVint(b)
	if err != nil {
		return 0, 0, err
	}
	return int64(v) - (int64(1)<<(7*n-1) - 1), n, nil
}

// Uint decodes an unsigned integer element's data.
func Uint(b []byte) uint64 {
	var v uint64
	for _, c := range b {
		v = v<<8 | uint64(c)
	}
	return v
}

// Int decodes a signed integer element's data.
func Int(b []byte) int64 {
	if len(b) == 0 {
		return 0
	}
	v := int64(int8(b[0]))
	for _, c := range b[1:] {
		v = v<<8 | int64(c)
	}
	return v
}

// Float decodes a float element's data (4 or 8 bytes; 0 bytes is 0).
func Float(b []byte) float64 {
	switch len(b) {
	case 4:
		return float64(math.Float32frombits(binary.BigEndian.Uint32(b)))
	case 8:
		return math.Float64frombits(binary.BigEndian.Uint64(b))
	}
	return 0
}

// String decodes a string element's data (trailing NULs are padding).
func String(b []byte) string {
	for len(b) > 0 && b[len(b)-1] == 0 {
		b = b[:len(b)-1]
	}
	return string(b)
}

// ---- writing ----

// AppendID appends an element ID.
func AppendID(b []byte, id uint32) []byte {
	switch {
	case id >= 1<<24:
		return append(b, byte(id>>24), byte(id>>16), byte(id>>8), byte(id))
	case id >= 1<<16:
		return append(b, byte(id>>16), byte(id>>8), byte(id))
	case id >= 1<<8:
		return append(b, byte(id>>8), byte(id))
	}
	return append(b, byte(id))
}

// AppendSize appends a size in the shortest form (width 0) or in exactly
// width bytes (1-8), for sizes written in place later.
func AppendSize(b []byte, size uint64, width int) []byte {
	n := width
	if n == 0 {
		n = 1
		for n < 8 && size >= uint64(1)<<(7*n)-1 {
			n++
		}
	}
	v := size | uint64(1)<<(7*n)
	for i := n - 1; i >= 0; i-- {
		b = append(b, byte(v>>(8*i)))
	}
	return b
}

// AppendElement appends an element with the given data.
func AppendElement(b []byte, id uint32, data []byte) []byte {
	b = AppendID(b, id)
	b = AppendSize(b, uint64(len(data)), 0)
	return append(b, data...)
}

// AppendUint appends an unsigned integer element in the fewest bytes.
func AppendUint(b []byte, id uint32, v uint64) []byte {
	n := 1
	for n < 8 && v >= uint64(1)<<(8*n) {
		n++
	}
	d := make([]byte, n)
	for i := n - 1; i >= 0; i-- {
		d[i] = byte(v)
		v >>= 8
	}
	return AppendElement(b, id, d)
}

// AppendUintWidth appends an unsigned integer element in exactly n bytes.
func AppendUintWidth(b []byte, id uint32, v uint64, n int) []byte {
	d := make([]byte, n)
	for i := n - 1; i >= 0; i-- {
		d[i] = byte(v)
		v >>= 8
	}
	return AppendElement(b, id, d)
}

// Void returns a Void element that takes exactly n bytes (n >= 2).
func Void(n int64) ([]byte, error) {
	if n < 2 {
		return nil, fmt.Errorf("a Void element needs at least 2 bytes, not %d", n)
	}
	// 1 byte of ID, w bytes of size, the rest data.
	for w := 1; w <= 8; w++ {
		data := n - 1 - int64(w)
		if data >= 0 && uint64(data) < uint64(1)<<(7*w)-1 {
			b := []byte{0xEC}
			b = AppendSize(b, uint64(data), w)
			return append(b, make([]byte, data)...), nil
		}
	}
	return nil, fmt.Errorf("cannot make a Void of %d bytes", n)
}
