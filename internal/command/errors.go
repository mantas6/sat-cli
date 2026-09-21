package command

import "fmt"

// ExitError asks the executable to return a specific process exit code.
type ExitError struct {
	Code int
	Err  error
}

// Error returns the underlying error text.
func (e ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit with code %d", e.Code)
	}
	return e.Err.Error()
}

// Unwrap exposes the underlying error.
func (e ExitError) Unwrap() error {
	return e.Err
}
