package config

import (
	// standard
	"os"
	"bufio"
	"fmt"
	"net"
	"strings"
	"errors"
	// external
	// local
)


// return a slice containing all DNS servers (IPv4) from
// a string or file.
// > parseServerList("8.8.8.8, 1.1.1.1") -> [8.8.8.8 1.1.1.1]
// > parseServerList("/tmp/srv.lst") -> [1.1.1.1 2.2.2.2 3.3.3.3 ...]
func ParseServerList(input string) ([]string, error) {
	var servers []string
	var scanner *bufio.Scanner

	if st, err := os.Stat(input); err == nil && !st.IsDir() {
		file, err := os.Open(input)
		if err != nil {
			return nil, fmt.Errorf("Can't open %q: %w", input, err)
		}
		defer file.Close()
		scanner = bufio.NewScanner(file)
	} else {
		scanner = bufio.NewScanner(strings.NewReader(input))
	}
	// for each line:
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "#"); idx != -1 {
			line = line[:idx]
		}
		// for each comma-separated elem:
		for _, elem := range strings.Split(line, ",") {
			elem = strings.TrimSpace(elem)
			if elem == "" {
				continue
			}
			if ip := net.ParseIP(elem); ip == nil {
				return nil, fmt.Errorf("Invalid IP: %q", elem)
			}
			servers = append(servers, elem)
		}
	}
	if len(servers) == 0 {
		return nil, errors.New("server list is empty")
	}
	return servers, nil
}
