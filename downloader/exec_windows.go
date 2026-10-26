package downloader

import (
	"os/exec"
	"syscall"
)

// createNoWindow stops a console window flashing up for ffmpeg when the
// downloader runs inside a GUI process.
const createNoWindow = 0x08000000

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
