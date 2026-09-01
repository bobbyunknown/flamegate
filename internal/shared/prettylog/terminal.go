package prettylog

import (
	"os"

	"github.com/mattn/go-isatty"
)

// IsTerminal reports whether the given file descriptor is connected to a
// terminal (including Windows Cygwin/Mintty/Git Bash).
func IsTerminal(fd uintptr) bool {
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// StdoutIsTTY is a convenience check for the common case.
func StdoutIsTTY() bool {
	return IsTerminal(os.Stdout.Fd())
}
