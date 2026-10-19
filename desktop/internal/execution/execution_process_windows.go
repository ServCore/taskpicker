//go:build windows

package execution

import (
	"os/exec"
	"strconv"
)

func configureExecutionProcess(command *exec.Cmd) {
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(command.Process.Pid)).Run()
	}
}
