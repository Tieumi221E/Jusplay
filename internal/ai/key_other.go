//go:build !windows

package ai

import "errors"

// Keys are only kept where the system can encrypt them (Windows DPAPI).
func protect(string) (string, error) { return "", errors.New("keys can only be stored on Windows") }

func unprotect(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	return "", errors.New("keys can only be read on Windows")
}
