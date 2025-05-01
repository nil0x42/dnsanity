package tty

import (
	// standard
	"fmt"
	"os"
	"regexp"
	"sync"
	"io"

	"golang.org/x/sys/unix"
    "golang.org/x/term"
)

// stripAnsiRegex removes ANSI escape sequences (colors, cursor movements, etc.).
var stripAnsiRegex = regexp.MustCompile(
	"[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))",
)

// cacheIsTTY stores the result of IsTTY for each file descriptor,
// avoiding repeated system calls.
var cacheIsTTY sync.Map // Key = file descriptor (uintptr), Value = bool


// IsTTY returns whether the given writer is a terminal.
// The result is cached to prevent repeated checks.
func IsTTY(w io.Writer) bool {
    f, ok := w.(*os.File)
    if !ok {
        return false
    }
    fd := f.Fd()
    if v, loaded := cacheIsTTY.Load(fd); loaded {
        return v.(bool)
    }
    isTerm := term.IsTerminal(int(fd))
    cacheIsTTY.Store(fd, isTerm)
    return isTerm
}

func OpenTTY() *os.File {
    fd, err := unix.Open("/dev/tty", unix.O_WRONLY|unix.O_NONBLOCK, 0)
    if err != nil {
        return nil
    }
	tty := os.NewFile(uintptr(fd), "/dev/tty")
	if !IsTTY(tty) {
        tty.Close()
        return nil
    }
    return tty
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
