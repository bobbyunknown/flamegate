package prettylog

import (
	"os"

	"github.com/mattn/go-isatty"
)

// IsTerminal reports whether the given file descriptor is connected to a
// terminal. Used to auto-enable pretty output during development while keeping
// plain text in CI/production (piped or redirected stdout).
func IsTerminal(fd uintptr) bool {
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// StdoutIsTTY is a convenience check for the common case.
func StdoutIsTTY() bool {
	return IsTerminal(os.Stdout.Fd())
}
