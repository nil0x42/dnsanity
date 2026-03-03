package dns

import (
	"context"
	"errors"
	"net"
	"syscall"
	"time"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
	"golang.org/x/sys/unix"
)

var dnsServerPort = "53"

func ResolveDNS(
	domain string,
	dnsServer string,
	timeout time.Duration,
	ctx context.Context,
) *DNSAnswer {
	transport := dns.NewTransport()
	transport.Dialer.Timeout = timeout
	transport.ReadTimeout = timeout
	transport.WriteTimeout = timeout
	client := &dns.Client{Transport: transport}

	message := dns.NewMsg(dnsutil.Fqdn(domain), dns.TypeA)
	message.UDPSize = 1232

	// init DNSAnswer
	answer := &DNSAnswer{Domain: domain}

	// DNS resolution
	// net.JoinHostPort() is needed for ipv6 (bracket expansion):
	hostAndPort := net.JoinHostPort(dnsServer, dnsServerPort)
	response, _, err := client.Exchange(ctx, message, "udp", hostAndPort)
	if err != nil {
		answer.Status = mapResolveError(err)
	} else if response.Rcode != dns.RcodeSuccess {
		answer.Status = dns.RcodeToString[response.Rcode]
	} else {
		for _, rr := range response.Answer {
			switch record := rr.(type) {
			case *dns.A:
				answer.A = append(answer.A, record.A.Addr.String())
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

func mapResolveError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "TIMEOUT"
	}
	if errors.Is(err, context.Canceled) {
		return "ERROR - " + err.Error()
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsTimeout {
			return "TIMEOUT"
		}
		if dnsErr.IsNotFound {
			return "ERROR - " + err.Error()
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "TIMEOUT"
	}

	var errno syscall.Errno
	if errors.As(err, &errno) {
		if errno == 0 {
			return "ERROR - " + err.Error()
		}
		if errno == syscall.ETIMEDOUT {
			return "TIMEOUT"
		}
		if name := unix.ErrnoName(errno); name != "" {
			return name
		}
		return errno.Error()
	}

	return "ERROR - " + err.Error()
}
