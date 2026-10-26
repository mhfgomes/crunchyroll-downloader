//go:build !windows

package downloader

import "os/exec"

func hideWindow(*exec.Cmd) {}
