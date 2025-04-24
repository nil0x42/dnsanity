package display

import (
	// standard
	"os"
	"fmt"
	// external
	// local
	"github.com/nil0x42/dnsanity/internal/dnsanitize"
)


func ReportValidResults(
	servers []dnsanitize.ServerContext,
	filepath string,
) {
	var f *os.File
	var err error
	if filepath == "" || filepath == "-" || filepath == "/dev/stdout" {
		f = os.Stdout
	} else {
		f, err = os.Create(filepath)
		if err != nil {
			fmt.Fprintf(
				os.Stderr, "Can't open output file: %q: %v\n", filepath, err,
			)
			os.Exit(1)
		}
		defer f.Close()
	}
	for _, srv := range servers {
		if srv.Disabled {
			continue
		}
		fmt.Fprintf(f, "%v\n", srv.IPAddress);
	}
}

func ReportAllResults(
	servers []dnsanitize.ServerContext,
	filepath string,
) {
	var f *os.File
	var err error
	if filepath == "" || filepath == "-" || filepath == "/dev/stdout" {
		f = os.Stdout
	} else {
		f, err = os.Create(filepath)
		if err != nil {
			fmt.Fprintf(
				os.Stderr, "Can't open output file: %q: %v\n", filepath, err,
			)
			os.Exit(1)
		}
		defer f.Close()
	}
	for _, srv := range servers {
		if srv.Disabled {
			fmt.Fprintf(f, "- %v\n", srv.IPAddress);
		} else {
			fmt.Fprintf(f, "+ %v\n", srv.IPAddress);
		}
	}
}
