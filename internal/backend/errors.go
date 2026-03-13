package backend

import "errors"

var (
	// ErrNotFound indicates the requested remote file does not exist.
	ErrNotFound = errors.New("remote file not found")

	// ErrAuth indicates an authentication or authorization failure.
	ErrAuth = errors.New("backend authentication failed")

	// ErrNetwork indicates a transient network error.
	ErrNetwork = errors.New("backend network error")
)
