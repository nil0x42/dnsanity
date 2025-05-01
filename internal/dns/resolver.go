package dns

import (
	"sort"
	"strings"
	"time"
	"context"

	"github.com/miekg/dns"
)


func ResolveDNS(
	domain		string,
	dnsServer	string,
	timeout		time.Duration,
	ctx			context.Context,
) *DNSAnswer {
	client := &dns.Client{
		Timeout: timeout,
		// UDPSize: 4096,
	}

	message := &dns.Msg{}
	message.SetQuestion(dns.Fqdn(domain), dns.TypeA) // A record
	message.SetEdns0(4096, false)

	// init DNSAnswer
	answer := &DNSAnswer{}
	answer.Domain = domain

	// DNS resolution
	response, _, err := client.ExchangeContext(ctx, message, dnsServer+":53")
	if err != nil {
		if strings.HasSuffix(err.Error(), "i/o timeout") {
			answer.Status = "TIMEOUT"
		} else if strings.HasSuffix(err.Error(), "read: connection refused") {
			answer.Status = "ECONNREFUSED"
		} else if strings.HasSuffix(err.Error(), "connect: network is unreachable") {
			answer.Status = "NO_INTERNET"
		} else {
			answer.Status = "ERROR - " + err.Error()
		}
	} else if response.Rcode != dns.RcodeSuccess {
		answer.Status = dns.RcodeToString[response.Rcode]
	} else {
		for _, rr := range response.Answer {
			switch record := rr.(type) {
			case *dns.A:
				answer.A = append(answer.A, record.A.String())
			case *dns.CNAME:
				answer.CNAME = append(answer.CNAME, record.Target)
			}
		}
		sort.Strings(answer.A)
		sort.Strings(answer.CNAME)
		// special case: aldkjasdlskj.invalid.com
		// 1.1.1.1 returns NOERROR, instead of SERVFAIL when there are no results but tld exists
		// so we force a NOERROR with no results to return SERVFAIL for consistence
		if len(answer.A) == 0 && len(answer.CNAME) == 0 {
			answer.Status = "SERVFAIL"
		} else {
			answer.Status = "NOERROR"
		}
	}
	return answer
}
