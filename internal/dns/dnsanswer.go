package dns

import (
	"fmt"
	"strings"
)

// --------------------------------------------------------------------
// DNSAnswerData
// --------------------------------------------------------------------

type DNSAnswerData struct {
	Status string   // NOERROR | NXDOMAIN | TIMEOUT | SERVFAIL
	A      []string // sorted A records (IPv4)
	CNAME  []string // sorted CNAME records
}

func (dad *DNSAnswerData) ToString() string {
	if len(dad.A) == 0 && len(dad.CNAME) == 0 {
		return dad.Status
	}
	// here, it's implicitly a NOERROR, because we got results..
	records := []string{}
	for _, a := range dad.A {
		records = append(records, "A="+a)
	}
	for _, cname := range dad.CNAME {
		records = append(records, "CNAME="+cname)
	}
	return strings.Join(records, " ")
}

func NewDNSAnswerData(data string) (*DNSAnswerData, error) {
	tokens := strings.Fields(data)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("empty answer")
	}
	dad := &DNSAnswerData{}
	// single 'STATUS' word:
	if len(tokens) == 1 {
		switch tokens[0] {
		case
			"TIMEOUT",
			"NOERROR",
			"FORMERR",
			"NOTIMP",
			"NXDOMAIN",
			"SERVFAIL":
			dad.Status = tokens[0]
			return dad, nil
		default:
		}
	}
	// 1 or more A / CNAME records (implicitly a NOERROR)
	for _, tok := range tokens {
		if strings.HasPrefix(tok, "A=") {
			dad.A = append(
				dad.A,
				strings.TrimPrefix(tok, "A="),
			)
		} else if strings.HasPrefix(tok, "CNAME=") {
			dad.CNAME = append(
				dad.CNAME,
				strings.ToLower(strings.TrimPrefix(tok, "CNAME=")),
			)
		} else {
			return nil, fmt.Errorf("invalid record: %q", tok)
		}
	}
	dad.Status = "NOERROR"
	return dad, nil
}

// --------------------------------------------------------------------
// DNSAnswer
// --------------------------------------------------------------------

type DNSAnswer struct {
	Domain string
	DNSAnswerData
	Truncated bool
}

// DNSAnswer.ToString converts a DNSAnswer to string
func (da *DNSAnswer) ToString() string {
	out := da.Domain + " " + da.DNSAnswerData.ToString()
	if da.Truncated {
		out += " [TC=1]"
	}
	return out
}

// IsWorthRetrying returns true if the answer is eligible for a retry.
// Criteria:
// - Transient DNS errors: TIMEOUT or SERVFAIL
// - Truncated successful response: NOERROR with Truncated == true (retry over TCP may succeed)
func (da *DNSAnswer) IsWorthRetrying() bool {
	if da == nil {
		return false
	}
	switch da.Status {
	case "TIMEOUT", "SERVFAIL":
		return true // transient, worth another shot
	case "NOERROR":
		// Retry when the response was truncated; a TCP retry may succeed.
		return da.Truncated
	default:
		return false // permanent or deterministic mismatch
	}
}
