//go:build !windows

package shell

import "errors"

type Window struct{}

func Open(title, url string, dark bool, profile string) (*Window, error) {
	return nil, errors.New("the player window needs Windows and WebView2")
}

func (*Window) Run()   {}
func (*Window) Close() {}
