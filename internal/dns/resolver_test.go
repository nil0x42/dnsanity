package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/rdata"
)

type timeoutNetError struct {
	err     error
	timeout bool
}

func (e timeoutNetError) Error() string   { return e.err.Error() }
func (e timeoutNetError) Timeout() bool   { return e.timeout }
func (e timeoutNetError) Temporary() bool { return false }
func (e timeoutNetError) Unwrap() error   { return e.err }

func startTestDNSServer(t *testing.T) (serverAddr string, serverPort string, shutdown func()) {
	t.Helper()

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		buf := make([]byte, 4096)
		for {
			_ = pc.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					select {
					case <-ctx.Done():
						return
					default:
						continue
					}
				}
				return
			}

			req := new(dns.Msg)
			req.Data = append(req.Data[:0], buf[:n]...)
			if err := req.Unpack(); err != nil || len(req.Question) == 0 {
				continue
			}

			qname := strings.ToLower(req.Question[0].Header().Name)
			resp := &dns.Msg{}
			resp.ID = req.ID
			resp.Response = true
			resp.Opcode = req.Opcode
			resp.RecursionDesired = req.RecursionDesired
			resp.Question = req.Question

			switch qname {
			case "example.com.":
				resp.Answer = []dns.RR{&dns.A{Hdr: dns.Header{Name: qname, Class: dns.ClassINET, TTL: 60}, A: rdata.A{Addr: netip.MustParseAddr("93.184.216.34")}}}
			case "www.example.com.":
				resp.Answer = []dns.RR{
					&dns.CNAME{Hdr: dns.Header{Name: qname, Class: dns.ClassINET, TTL: 60}, CNAME: rdata.CNAME{Target: "example.com."}},
					&dns.A{Hdr: dns.Header{Name: "example.com.", Class: dns.ClassINET, TTL: 60}, A: rdata.A{Addr: netip.MustParseAddr("93.184.216.34")}},
				}
			case "nxdomain.example.":
				resp.Rcode = dns.RcodeNameError
			case "servfail.example.":
				resp.Rcode = dns.RcodeServerFailure
			case "truncated.example.":
				resp.Answer = []dns.RR{&dns.A{Hdr: dns.Header{Name: qname, Class: dns.ClassINET, TTL: 60}, A: rdata.A{Addr: netip.MustParseAddr("203.0.113.10")}}}
				resp.Truncated = true
			default:
				resp.Rcode = dns.RcodeNameError
			}

			if err := resp.Pack(); err != nil {
				continue
			}
			_, _ = pc.WriteTo(resp.Data, addr)
		}
	}()

	addr := pc.LocalAddr().(*net.UDPAddr)

	return addr.IP.String(), strconv.Itoa(addr.Port), func() {
		cancel()
		_ = pc.Close()
	}
}

func TestResolveDNS(t *testing.T) {
	srvAddr, srvPort, shutdown := startTestDNSServer(t)
	defer shutdown()

	oldPort := dnsServerPort
	dnsServerPort = srvPort
	defer func() { dnsServerPort = oldPort }()

	tests := []struct {
		name         string
		domain       string
		server       string
		port         string
		timeout      time.Duration
		cancelCtx    bool
		wantStatuses []string
		wantA        bool
		wantCNAME    bool
		wantTC       bool
	}{
		{name: "SuccessARecord", domain: "example.com", server: srvAddr, port: srvPort, timeout: time.Second, wantStatuses: []string{"NOERROR"}, wantA: true},
		{name: "SuccessCNAME", domain: "www.example.com", server: srvAddr, port: srvPort, timeout: time.Second, wantStatuses: []string{"NOERROR"}, wantA: true, wantCNAME: true},
		{name: "NXDOMAIN", domain: "nxdomain.example", server: srvAddr, port: srvPort, timeout: time.Second, wantStatuses: []string{"NXDOMAIN"}},
		{name: "SERVFAIL", domain: "servfail.example", server: srvAddr, port: srvPort, timeout: time.Second, wantStatuses: []string{"SERVFAIL"}},
		{name: "TruncatedNOERROR", domain: "truncated.example", server: srvAddr, port: srvPort, timeout: time.Second, wantStatuses: []string{"NOERROR"}, wantA: true, wantTC: true},
		{name: "Timeout", domain: "example.com", server: "192.0.2.1", port: "53", timeout: 50 * time.Millisecond, wantStatuses: []string{"TIMEOUT", "ENETUNREACH", "EHOSTUNREACH"}},
		{name: "ConnectionRefused", domain: "example.com", server: "127.0.0.1", port: "1", timeout: 100 * time.Millisecond, wantStatuses: []string{"ECONNREFUSED"}},
		{name: "InvalidServer", domain: "example.com", server: "[", port: "53", timeout: 100 * time.Millisecond, wantStatuses: []string{"ERROR - "}},
		{name: "ContextCanceled", domain: "example.com", server: srvAddr, port: srvPort, timeout: time.Second, cancelCtx: true, wantStatuses: []string{"ERROR - context canceled", "ERROR - dial udp "}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			if tc.cancelCtx {
				cancel()
			} else {
				defer cancel()
			}

			dnsServerPort = tc.port

			ans := ResolveDNS(tc.domain, tc.server, tc.timeout, ctx)

			matched := false
			for _, wantStatus := range tc.wantStatuses {
				if strings.Contains(ans.Status, wantStatus) {
					matched = true
					break
				}
			}
			if !matched {
				t.Fatalf("status mismatch: got %q want one of %v", ans.Status, tc.wantStatuses)
			}
			if ans.Domain != tc.domain {
				t.Fatalf("domain mismatch: got %q want %q", ans.Domain, tc.domain)
			}
			if tc.wantA && len(ans.A) == 0 {
				t.Fatalf("expected at least one A record")
			}
			if tc.wantCNAME && len(ans.CNAME) == 0 {
				t.Fatalf("expected at least one CNAME record")
			}
			if tc.wantTC != ans.Truncated {
				t.Fatalf("truncated mismatch: got %v want %v", ans.Truncated, tc.wantTC)
			}
		})
	}
}

func TestMapResolveError(t *testing.T) {
	t.Parallel()

	dnsTimeoutErr := &net.DNSError{Err: "i/o timeout", IsTimeout: true}
	dnsNotFoundErr := &net.DNSError{Err: "no such host", IsNotFound: true, Name: "bad.example"}
	opTimeoutErr := &net.OpError{Op: "read", Net: "udp", Err: os.ErrDeadlineExceeded}
	opRefusedErr := &net.OpError{Op: "dial", Net: "udp", Err: syscall.ECONNREFUSED}
	wrappedRefusedErr := fmt.Errorf("resolve failed: %w", opRefusedErr)
	wrappedHostUnreachErr := fmt.Errorf("wrapper: %w", syscall.EHOSTUNREACH)
	wrappedUnknownErrno := fmt.Errorf("wrapper: %w", syscall.Errno(12345))

	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "Nil", err: nil, want: ""},
		{name: "ContextDeadline", err: context.DeadlineExceeded, want: "TIMEOUT"},
		{name: "ContextCanceled", err: context.Canceled, want: "ERROR - context canceled"},
		{name: "WrappedContextDeadline", err: fmt.Errorf("wrapped: %w", context.DeadlineExceeded), want: "TIMEOUT"},
		{name: "DNSErrorTimeout", err: dnsTimeoutErr, want: "TIMEOUT"},
		{name: "DNSErrorNotFound", err: dnsNotFoundErr, want: "ERROR - lookup bad.example: no such host"},
		{name: "NetErrorTimeoutNoErrno", err: timeoutNetError{err: errors.New("socket timeout"), timeout: true}, want: "TIMEOUT"},
		{name: "OpErrorDeadlineExceeded", err: opTimeoutErr, want: "TIMEOUT"},
		{name: "ErrnoConnRefused", err: syscall.ECONNREFUSED, want: "ECONNREFUSED"},
		{name: "WrappedErrnoConnRefused", err: wrappedRefusedErr, want: "ECONNREFUSED"},
		{name: "WrappedErrnoHostUnreachable", err: wrappedHostUnreachErr, want: "EHOSTUNREACH"},
		{name: "ErrnoTimedOut", err: syscall.ETIMEDOUT, want: "TIMEOUT"},
		{name: "UnknownErrnoFallback", err: wrappedUnknownErrno, want: "errno 12345"},
		{name: "PlainErrorFallback", err: errors.New("boom"), want: "ERROR - boom"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := mapResolveError(tc.err); got != tc.want {
				t.Fatalf("mapResolveError(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}
