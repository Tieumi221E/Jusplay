//go:build !windows

package ai

import (
	"os"
	"os/exec"
)

func hide(*exec.Cmd)    {}
func adopt(*os.Process) {}
