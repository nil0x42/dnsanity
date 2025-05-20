package report

import (
	"os"
	"io"
)

type IOFiles struct {
	TTYFile		*os.File
	OutputFile	io.Writer
	VerboseFile	io.Writer
	DebugFile	io.Writer
}
