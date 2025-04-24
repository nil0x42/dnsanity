package dns

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

type DNSAnswer struct {
	Domain string
	Status string   // NOERROR | NXDOMAIN | TIMEOUT | SERVFAIL
	A      []string // sorted A records (IPv4)
	CNAME  []string // sorted CNAME records
}

// DNSAnswer.ToString converts a DNSAnswer to string
func (da *DNSAnswer) ToString() string {
	if len(da.A) == 0 && len(da.CNAME) == 0 {
		return fmt.Sprintf("%s %s", da.Domain, da.Status)
	}
	// here, it's implicitly a NOERROR, because we got results..
	records := []string{}
	for _, a := range da.A {
		records = append(records, "A="+a)
	}
	for _, cname := range da.CNAME {
		records = append(records, "CNAME="+cname)
	}
	return fmt.Sprintf("%s %s", da.Domain, strings.Join(records, " "))
}

// DNSANswer.Equals() compares itself to another DNSAnswer
func (da *DNSAnswer) Equals(other *DNSAnswer) bool {
	if da == nil || other == nil {
		return false
	}
	if da.Domain != other.Domain {
		return false
	}
	if da.Status != other.Status {
		// Be nice to the large amount of servers who timeout instead
		// of returning expected SERVFAIL for existing TLDs without records:
		if !((da.Status == "TIMEOUT" && other.Status == "SERVFAIL") ||
			(da.Status == "SERVFAIL" && other.Status == "TIMEOUT")) {
			return false
		}
	}
	if !matchRecords(da.A, other.A) {
		return false
	}
	if !matchRecords(da.CNAME, other.CNAME) {
		return false
	}

	return true
}

// matchRecords compares two slices of records using glob matching
// Returns true if each record in patterns matches exactly one record in values
func matchRecords(patterns, values []string) bool {
	if len(patterns) != len(values) {
		return false
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
			if !matchRecord(pattern, values[perm[i]]) {
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

// matchRecord compares a pattern with a value using glob matching
// Returns true if the value matches the pattern
// fmt.Println(matchRecord("192.168.*.*", "192.168.1.1")) // true
// fmt.Println(matchRecord("1.1.*.1", "1.1.10.1"))        // true
// fmt.Println(matchRecord("10.*.0.1", "10.200.0.1"))     // true
// fmt.Println(matchRecord("192.168.1.*", "192.168.2.1")) // false
// fmt.Println(matchRecord("*.*.*.*", "255.255.255.255")) // true
// fmt.Println(matchRecord("*.example.com", "test.example.com")) // true
// fmt.Println(matchRecord("*.example.com", "sub.test.com")) // false
func matchRecord(pattern, str string) bool {
	str = strings.ToLower(str) // case insensitive
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == str
	}
	if !strings.HasPrefix(str, parts[0]) {
		return false
	}
	if !strings.HasSuffix(str, parts[len(parts)-1]) {
		return false
	}
	return true
}

var errNoEntries = errors.New("no entries found")

// new DNSAnswer from string
func DNSAnswerFromString(input string) (*DNSAnswer, error) {
	parts := strings.Fields(input)
	if len(parts) < 2 {
		return nil, fmt.Errorf("must have a domain and at least one A|CNAME record or NXDOMAIN/NOERROR")
	}
	answer := &DNSAnswer{}
	answer.Domain = parts[0]

	records := parts[1:]
	if len(records) == 1 {
		switch records[0] {
		case "NXDOMAIN", "SERVFAIL":
			answer.Status = records[0]
			return answer, nil
		default:
		}
	}
	for _, rec := range records {
		if strings.HasPrefix(rec, "A=") {
			answer.A = append(answer.A, strings.TrimPrefix(rec, "A="))
		} else if strings.HasPrefix(rec, "CNAME=") {
			answer.CNAME = append(answer.CNAME, strings.ToLower(strings.TrimPrefix(rec, "CNAME=")))
		} else {
			return nil, fmt.Errorf("invalid record: %q", rec)
		}
	}
	// make sure there is at least 1 A or CNAME:
	if len(answer.A) == 0 && len(answer.CNAME) == 0 {
		return nil, fmt.Errorf("must contain at least one A or CNAME record")
	}
	// sort records for deterministic comparison
	sort.Strings(answer.A)
	sort.Strings(answer.CNAME)
	answer.Status = "NOERROR" // default status is NOERROR
	return answer, nil
}

// load a template ([]DNSAnswer) from file.
func DNSAnswerSliceFromFile(filePath string) ([]DNSAnswer, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("%q: %w", filePath, err)
	}
	defer file.Close()

	answers, err := parseDNSAnswers(file, func(err error, lineNo int) error {
		return fmt.Errorf("%v line %v: %w", filePath, lineNo, err)
	})
	if err != nil {
		if err == errNoEntries {
			return nil, fmt.Errorf("Can't find any entry")
		}
		return nil, fmt.Errorf("Can't read %q: %w", filePath, err)
	}
	return answers, nil
}

// load a template ([]DNSAnswer) from a multiline string.
func DNSAnswerSliceFromString(input string) ([]DNSAnswer, error) {
	answers, err := parseDNSAnswers(strings.NewReader(input), func(err error, lineNo int) error {
		return fmt.Errorf("line %v: %w", lineNo, err)
	})
	if err != nil {
		if err == errNoEntries {
			return nil, fmt.Errorf("Can't find any entry")
		}
		return nil, fmt.Errorf("Error reading input: %w", err)
	}
	return answers, nil
}

// parseDNSAnswers reads DNS answers from a reader, using wrapErr to format line-specific errors
func parseDNSAnswers(r io.Reader, wrapErr func(error, int) error) ([]DNSAnswer, error) {
	var answers []DNSAnswer
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
		answer, err := DNSAnswerFromString(line)
		if err != nil {
			return nil, wrapErr(err, lineNo)
		}
		answers = append(answers, *answer)
		lineNo++
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(answers) == 0 {
		return nil, errNoEntries
	}
	return answers, nil
}
