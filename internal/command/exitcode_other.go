//go:build !unix

package command

import "os/exec"

// signalExitCode has no portable signal information on non-unix platforms, so
// callers fall back to the conventional 130 exit code.
func signalExitCode(*exec.ExitError) (int, bool) {
	return 130, true
}
