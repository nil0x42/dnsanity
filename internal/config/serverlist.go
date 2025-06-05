package config

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
)

// ParseServerList parses input and returns the DNS server IP addresses it
// contains. The input may be a comma‑separated string or a path to a file and
// supports both IPv4 and IPv6 addresses.
//
// Example:
//
//	ParseServerList("8.8.8.8, 1.1.1.1")
//	ParseServerList("/tmp/srv.lst")
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
