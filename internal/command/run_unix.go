//go:build unix

package command

import (
	"os/exec"
	"syscall"
)

// signalExitCode returns the conventional 128+signal exit code when the
// process was terminated by a signal, using the unix WaitStatus.
func signalExitCode(exitErr *exec.ExitError) (int, bool) {
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() {
		return 0, false
	}
	return 128 + int(status.Signal()), true
}
