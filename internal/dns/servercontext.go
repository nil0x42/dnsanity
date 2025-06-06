package dns

import (
	"time"
	"context"
	"fmt"
)

type CheckContext struct {
	Answer              *DNSAnswer      // last received answer
	Passed              bool           // last attempt result
	AttemptsLeft        int            // retries remaining
	MaxAttempts         int            // immutable upper bound
}

type ServerContext struct {
	Ctx                 context.Context
    CancelCtx           context.CancelFunc
	Disabled            bool           // true if reaches maxFailures

	IPAddress           string         // resolver IPv4
	FailedCount         int            // failed checks.
	CompletedCount      int            // finished checks (pass+fail)
	NextQueryAt         time.Time      // honour per-server rps
	PendingChecks       []int          // queue of remaining check indexes
	Checks              []CheckContext // answers log
}

func NewServerContext(
	ipAddress	string,
	template	Template,
	maxAttempts	int, // max attempts per check
) *ServerContext {
	ctx, cancelCtx := context.WithCancel(context.Background())
	sc := &ServerContext{
		Ctx:			ctx,
		CancelCtx:		cancelCtx,
		IPAddress:      ipAddress,
		PendingChecks:  make([]int, len(template)),
		Checks:         make([]CheckContext, len(template)),
	}
	for i := range template {
		sc.PendingChecks[i] = i
		sc.Checks[i].AttemptsLeft = maxAttempts
		sc.Checks[i].MaxAttempts = maxAttempts
		sc.Checks[i].Answer = &DNSAnswer{
			Domain: template[i].Domain,
			DNSAnswerData: DNSAnswerData{Status: "SKIPPED"},
		}
	}
	return sc
}

// Finished returns true when the server is either disabled or has
// completed all its checks.
// ServerContext.Finished():
func (srv *ServerContext) Finished() bool {
	return srv.Disabled || srv.CompletedCount == len(srv.Checks)
}

func (srv *ServerContext) PrettyDump() string {
	var s string
	if srv.FailedCount == 0 {
		s += fmt.Sprintf(
			"\033[1;32m[+] SERVER %v (valid)\033[m\n", srv.IPAddress)
	} else {
		s += fmt.Sprintf(
			"\033[1;31m[-] SERVER %v (invalid)\033[m\n", srv.IPAddress)
	}
	for _, test := range srv.Checks {
		var prefix string
		if test.Passed {
			prefix = "\033[1;32m+\033[0;32m"
		} else if test.Answer.Status == "SKIPPED" {
			prefix = "\033[1;90m!\033[0;90m"
		} else {
			prefix = "\033[1;31m-\033[0;31m"
		}
		numTries := test.MaxAttempts - test.AttemptsLeft
		attemptsRepr := ""
		if numTries > 1 {
			suffix := "th"
			if numTries == 2 {
				suffix = "nd"
			} else if numTries == 3 {
				suffix = "rd"
			}
			attemptsRepr = fmt.Sprintf(
				" \033[33m(on %v%v attempt)\033[m", numTries, suffix)
		}
		s += fmt.Sprintf(
			"    %s %s\033[m%v\n",
			prefix, test.Answer.ToString(), attemptsRepr,
		)
	}
	return s
}
