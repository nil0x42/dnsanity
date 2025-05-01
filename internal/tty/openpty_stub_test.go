//go:build !((darwin || freebsd || openbsd || netbsd || dragonfly) && cgo)

package tty

import "os"

// openPTY stub : retourne une erreur sur les plateformes sans unix.Openpty
// (Linux, Windows, ou quand CGO est désactivé).
func openPTY() (*os.File, *os.File, error) {
	return nil, nil, os.ErrNotExist
}

