//go:build !unix

package command

import "os/exec"

// signalExitCode reports no signal on non-unix platforms, where
// ExitCode always carries the child's exit status.
func signalExitCode(*exec.ExitError) (int, bool) {
	return 0, false
}
