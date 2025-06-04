package dns

import (
	"strings"
	"time"
	"context"
	"net"

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
	message.SetQuestion(dns.Fqdn(domain), dns.TypeA) // A record

	// init DNSAnswer
	answer := &DNSAnswer{Domain: domain}

	// DNS resolution
	// net.JoinHostPort() is needed for ipv6 (bracket expansion):
	hostAndPort := net.JoinHostPort(dnsServer, "53") 
	response, _, err := client.ExchangeContext(ctx, message, hostAndPort)
	if err != nil {
		if strings.HasSuffix(err.Error(), "i/o timeout") {
			answer.Status = "TIMEOUT"
		} else if strings.HasSuffix(err.Error(), "read: connection refused") {
			answer.Status = "ECONNREFUSED"
		} else if strings.HasSuffix(err.Error(), "read: no route to host") {
			answer.Status = "EHOSTUNREACH"
		} else if strings.HasSuffix(err.Error(), "connect: network is unreachable") {
			answer.Status = "ENETUNREACH (no internet)"
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
	if err == nil { // check needed to avoid segfault if resp is not built
		answer.Truncated = response.Truncated
	}
	return answer
}
