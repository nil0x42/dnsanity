package dns

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// --------------------------------------------------------------------
// TemplateEntry (single template line/entry)
// --------------------------------------------------------------------
type TemplateEntry struct {
	Domain       string
	ValidAnswers []DNSAnswerData
}

// NewTemplateEntry() creates a new TemplateEntry from string
func NewTemplateEntry(line string) (*TemplateEntry, error) {
	// 1) Extract domain (first field) and remainder.
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return nil, fmt.Errorf("must have a domain and at least one A|CNAME record or NXDOMAIN/NOERROR")
	}
	domain := parts[0]
	remainder := line[len(domain):]

	// 2) Build entry holder.
	te := &TemplateEntry{Domain: domain}

	// 3) For each alternative separated by "||", build a DNSAnswerData.
	for _, alt := range strings.Split(remainder, "||") {
		answer, err := NewDNSAnswerData(strings.TrimSpace(alt))
		if err != nil {
			return nil, err
		}
		te.ValidAnswers = append(te.ValidAnswers, *answer)
	}
	return te, nil
}

func (te *TemplateEntry) ToString() string {
	altList := []string{}
	for _, dad := range te.ValidAnswers {
		altList = append(altList, dad.ToString())
	}
	return te.Domain + " " + strings.Join(altList, " || ")
}

// TemplateEntry.Matches() compares itself to a DNSAnswer
func (te *TemplateEntry) Matches(da *DNSAnswer) bool {
	if te != nil && da != nil && te.Domain == da.Domain {
		for _, choice := range te.ValidAnswers {
			if choice.Status == da.Status &&
				matchRecords(choice.A, da.A) &&
				matchRecords(choice.CNAME, da.CNAME) {
				return true
			}
		}
	}
	return false
}

// matchRecords compares two slices of records using glob matching
// Returns true if each record in patterns matches exactly one
// record in values, no matter the order
func matchRecords(patterns, values []string) bool {
	if len(patterns) != len(values) {
		return false // two slices with different sizes are not equal
	}
	if len(patterns) == 0 && len(values) == 0 {
		return true // two empty slices are equal
	}

	valueCounts := make(map[string]int, len(values))
	for _, value := range values {
		valueCounts[strings.ToLower(value)]++
	}

	// Phase 1: consume exact patterns first (no '*').
	// This is the hot path and avoids unnecessary glob matching work.
	globs := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.ToLower(pattern)
		if !strings.ContainsRune(pattern, '*') {
			if valueCounts[pattern] == 0 {
				return false
			}
			valueCounts[pattern]--
			continue
		}
		globs = append(globs, pattern)
	}

	if len(globs) == 0 {
		return true
	}

	// Phase 2: build the remaining values after exact matches have been consumed.
	// Only those values are candidates for glob matching.
	remainingValues := make([]string, 0, len(globs))
	for value, count := range valueCounts {
		for ; count > 0; count-- {
			remainingValues = append(remainingValues, value)
		}
	}
	if len(remainingValues) != len(globs) {
		return false
	}

	// Phase 3: solve the glob-to-value assignment on the reduced set
	// using bipartite matching (DFS augmenting paths).
	adj := make([][]int, len(globs))
	for i, pattern := range globs {
		for j, value := range remainingValues {
			if globMatch(pattern, value) {
				adj[i] = append(adj[i], j)
			}
		}
		if len(adj[i]) == 0 {
			return false
		}
	}

	valueToPattern := make([]int, len(remainingValues))
	for i := range valueToPattern {
		valueToPattern[i] = -1
	}

	for patternIdx := range globs {
		visited := make([]bool, len(remainingValues))
		if !tryAugment(patternIdx, adj, visited, valueToPattern) {
			return false
		}
	}
	return true
}

// tryAugment tries to assign one pattern node to a value node in the
// bipartite graph, potentially rerouting existing assignments recursively.
func tryAugment(patternIdx int, adj [][]int, visited []bool, valueToPattern []int) bool {
	for _, valueIdx := range adj[patternIdx] {
		if visited[valueIdx] {
			continue
		}
		visited[valueIdx] = true
		if valueToPattern[valueIdx] == -1 || tryAugment(valueToPattern[valueIdx], adj, visited, valueToPattern) {
			valueToPattern[valueIdx] = patternIdx
			return true
		}
	}
	return false
}

// globMatch compares a pattern with a value using glob matching
// Returns true if the value matches the pattern
// fmt.Println(globMatch("192.168.*.*", "192.168.1.1")) // true
// fmt.Println(globMatch("1.1.*.1", "1.1.10.1"))        // true
// fmt.Println(globMatch("10.*.0.1", "10.200.0.1"))     // true
// fmt.Println(globMatch("192.168.1.*", "192.168.2.1")) // false
// fmt.Println(globMatch("*.*.*.*", "255.255.255.255")) // true
// fmt.Println(globMatch("*.example.com", "test.example.com")) // true
// fmt.Println(globMatch("*.example.com", "sub.test.com")) // false
func globMatch(pattern, str string) bool {
	pattern = strings.ToLower(pattern) // case insensitive
	str = strings.ToLower(str)         // case insensitive
	if !strings.ContainsRune(pattern, '*') {
		return pattern == str // most common case (no glob)
	}
	// cut pattern on '*'
	parts := strings.Split(pattern, "*")

	// first segment
	if first := parts[0]; first != "" {
		if !strings.HasPrefix(str, first) {
			return false
		}
		str = str[len(first):]
	}
	// internediate segments
	for i := 1; i < len(parts)-1; i++ {
		part := parts[i]
		if part == "" { // several consecutive '*'
			continue
		}
		idx := strings.Index(str, part)
		if idx == -1 {
			return false
		}
		// cut str after found segment to preserve order
		str = str[idx+len(part):]
	}
	// last segment
	last := parts[len(parts)-1]
	if last != "" && !strings.HasSuffix(str, last) {
		return false
	}
	return true
}

// --------------------------------------------------------------------
// Template (list of template entries)
// --------------------------------------------------------------------
type Template []TemplateEntry

func (t Template) PrettyDump() string {
	out := "\033[1;34m[*] DNSANITY TEMPLATE:\033[m\n"
	for _, entry := range t {
		out += "    \033[34m* " + entry.ToString() + "\033[m\n"
	}
	return out
}

var errNoEntries = errors.New("no entries found")

// load a template ([]DNSAnswer) from file.
func NewTemplateFromFile(filePath string) (Template, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("%q: %w", filePath, err)
	}
	defer file.Close()

	tpl, err := loadTemplate(
		file,
		func(err error, lineNo int) error {
			return fmt.Errorf("%v line %v: %w", filePath, lineNo, err)
		},
	)
	if err != nil {
		if err == errNoEntries {
			return nil, fmt.Errorf("Can't find any entry")
		}
		return nil, fmt.Errorf("Can't read %q: %w", filePath, err)
	}
	return tpl, nil
}

// load a template ([]DNSAnswer) from a multiline string.
func NewTemplate(content string) (Template, error) {
	tpl, err := loadTemplate(
		strings.NewReader(content),
		func(err error, lineNo int) error {
			return fmt.Errorf("line %v: %w", lineNo, err)
		},
	)
	if err != nil {
		if err == errNoEntries {
			return nil, fmt.Errorf("Can't find any entry")
		}
		return nil, fmt.Errorf("Error reading input: %w", err)
	}
	return tpl, nil
}

// loadTemplate reads template entries from a reader,
// using wrapErr to format line-specific errors
func loadTemplate(
	r io.Reader,
	wrapErr func(error, int) error,
) (Template, error) {
	var tpl Template
	scanner := bufio.NewScanner(r)
	lineNo := 1
	for scanner.Scan() {
		line := scanner.Text()
		lineNoCurrent := lineNo
		lineNo++
		line = strings.Split(line, "#")[0]
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Convert to DNSAnswer
		entry, err := NewTemplateEntry(line)
		if err != nil {
			return nil, wrapErr(err, lineNoCurrent)
		}
		tpl = append(tpl, *entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	} else if len(tpl) == 0 {
		return nil, errNoEntries
	}
	return tpl, nil
}
