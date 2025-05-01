//go:build (darwin || freebsd || openbsd || netbsd || dragonfly) && cgo

package tty

import (
	"os"

	"golang.org/x/sys/unix"
)

// openPTY returns the master/slave *os.File pair using unix.Openpty.
// Compilé uniquement sur les OS où cette fonction existe et quand cgo est actif.
func openPTY() (*os.File, *os.File, error) {
	m, s, err := unix.Openpty(nil, nil)
	if err != nil {
		return nil, nil, err
	}
	return os.NewFile(uintptr(m), "ptmx"),
		os.NewFile(uintptr(s), "pts"), nil
}

