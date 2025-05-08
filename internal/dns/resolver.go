package dns

import (
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
	message.SetEdns0(1232, false)
	// // message.SetEdns0(4096, false)
	// message.SetEdns0(65535, false)
	message.SetQuestion(dns.Fqdn(domain), dns.TypeA) // A record

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
			answer.Status = "ENETUNREACH (no internet)"
			// answer.Status = "NO_INTERNET"
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
		answer.Status = "NOERROR"
	}
	if err == nil {
		answer.Truncated = response.Truncated
	}
	return answer
}
