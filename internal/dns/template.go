package dns

import (
	"fmt"
	"strings"
	"errors"
	"io"
	"os"
	"bufio"
)


// --------------------------------------------------------------------
// TemplateEntry (single template line/entry)
// --------------------------------------------------------------------
type TemplateEntry struct {
	Domain			string
	ValidAnswers	[]DNSAnswerData
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
	for _, alt := range strings.Split(remainder, "||" ) {
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
			if
			choice.Status == da.Status &&
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
	// Try all possible permutations of matching patterns to values
	perm := make([]int, len(values))
	for i := range perm {
		perm[i] = i
	}
	// Try each permutation
	for {
		// Check if this permutation works
		allMatch := true
		for i, pattern := range patterns {
			if !globMatch(pattern, values[perm[i]]) {
				allMatch = false
				break
			}
		}
		if allMatch {
			return true
		}
		// Get next permutation
		if !nextPermutation(perm) {
			break
		}
	}
	return false
}


// nextPermutation generates the next lexicographic permutation of the slice
// Returns false if there are no more permutations
func nextPermutation(p []int) bool {
	// Find the largest index k such that p[k] < p[k+1]
	k := len(p) - 2
	for k >= 0 && p[k] >= p[k+1] {
		k--
	}
	if k < 0 {
		return false
	}
	// Find the largest index l greater than k such that p[k] < p[l]
	l := len(p) - 1
	for p[k] >= p[l] {
		l--
	}
	// Swap p[k] and p[l]
	p[k], p[l] = p[l], p[k]
	// Reverse the sequence from p[k+1] up to and including the final element
	for i, j := k+1, len(p)-1; i < j; i, j = i+1, j-1 {
		p[i], p[j] = p[j], p[i]
	}
	return true
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
	str = strings.ToLower(str) // case insensitive
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
	r							io.Reader,
	wrapErr func(error, int)	error,
) (Template, error) {
	var tpl Template
	scanner := bufio.NewScanner(r)
	lineNo := 1
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.Split(line, "#")[0]
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Convert to DNSAnswer
		entry, err := NewTemplateEntry(line)
		if err != nil {
			return nil, wrapErr(err, lineNo)
		}
		tpl = append(tpl, *entry)
		lineNo++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	} else if len(tpl) == 0 {
		return nil, errNoEntries
	}
	return tpl, nil
}
