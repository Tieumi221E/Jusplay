package ai

import (
	"encoding/base64"
	"errors"
	"syscall"
	"unsafe"
)

// Service keys are kept encrypted with DPAPI (CryptProtectData): readable
// only by this Windows user on this computer, so a copied settings folder
// does not carry a usable key.

var (
	crypt32         = syscall.NewLazyDLL("crypt32.dll")
	cryptProtect    = crypt32.NewProc("CryptProtectData")
	cryptUnprotect  = crypt32.NewProc("CryptUnprotectData")
	localFree       = kernel32.NewProc("LocalFree")
	errDPAPI        = errors.New("Windows could not encrypt or decrypt the key")
	entropy         = []byte("Jusplay service key")
	cryptUIForbided = uintptr(0x1) // CRYPTPROTECT_UI_FORBIDDEN
)

type blob struct {
	n uint32
	p *byte
}

func newBlob(b []byte) *blob {
	if len(b) == 0 {
		return &blob{}
	}
	return &blob{uint32(len(b)), &b[0]}
}

func (b *blob) bytes() []byte {
	out := make([]byte, b.n)
	copy(out, unsafe.Slice(b.p, b.n))
	return out
}

func protect(key string) (string, error) {
	var out blob
	r, _, _ := cryptProtect.Call(uintptr(unsafe.Pointer(newBlob([]byte(key)))), 0, uintptr(unsafe.Pointer(newBlob(entropy))), 0, 0,
		cryptUIForbided, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return "", errDPAPI
	}
	defer localFree.Call(uintptr(unsafe.Pointer(out.p)))
	return base64.StdEncoding.EncodeToString(out.bytes()), nil
}

func unprotect(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	enc, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	var out blob
	r, _, _ := cryptUnprotect.Call(uintptr(unsafe.Pointer(newBlob(enc))), 0, uintptr(unsafe.Pointer(newBlob(entropy))), 0, 0,
		cryptUIForbided, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return "", errors.New("the key was stored by another Windows user or computer; enter it again")
	}
	defer localFree.Call(uintptr(unsafe.Pointer(out.p)))
	return string(out.bytes()), nil
}
