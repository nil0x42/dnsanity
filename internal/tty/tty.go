package tty

import (
	// standard
	"os"
	"fmt"
	"sync"
	"regexp"
	"syscall"
	"unsafe"
)

// stripAnsiRegex removes ANSI escape sequences (colors, cursor movements, etc.).
var stripAnsiRegex = regexp.MustCompile(
	"[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))",
)

// cacheIsTTY stores the result of IsTTY for each file descriptor,
// avoiding repeated system calls.
var cacheIsTTY sync.Map // Key = file descriptor (uintptr), Value = bool

// IsTTY returns whether the given file is a terminal.
// The result is cached to prevent repeated checks.
func IsTTY(f *os.File) bool {
	fd := f.Fd()
	if cached, ok := cacheIsTTY.Load(fd); ok {
		return cached.(bool)
	}
    var termios syscall.Termios
    _, _, err := syscall.Syscall6(
        syscall.SYS_IOCTL,
        uintptr(fd),
        uintptr(syscall.TCGETS),
        uintptr(unsafe.Pointer(&termios)),
        0, 0, 0,
    )
	isTerminal := err == 0
	cacheIsTTY.Store(fd, isTerminal)
	return isTerminal
}

// StripAnsi removes all ANSI escape sequences from a string.
func StripAnsi(str string) string {
	return stripAnsiRegex.ReplaceAllString(str, "")
}

// SmartFprintf behaves like fmt.Fprintf, but automatically strips
// ANSI sequences if the output file is not a TTY.
func SmartFprintf(f *os.File, format string, args ...interface{}) (int, error) {
	output := fmt.Sprintf(format, args...)
	if !IsTTY(f) {
		output = StripAnsi(output)
	}
	return f.WriteString(output)
}
