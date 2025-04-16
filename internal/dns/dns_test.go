package dns

import (
	"reflect"
	"strings"
	"testing"
)

// TestDNSAnswerFromString checks the parsing logic of DNSAnswerFromString.
func TestDNSAnswerFromString(t *testing.T) {
	tests := []struct {
		line       string
		wantErr    bool
		wantA      int
		wantCNAME  int
		wantStatus string
	}{
		{"example.com A=1.2.3.4", false, 1, 0, "NOERROR"},
		{"example.org NXDOMAIN", false, 0, 0, "NXDOMAIN"},
		{"cr.yp.to A=131.193.32.108 A=131.193.32.109", false, 2, 0, "NOERROR"},
		{"wiki.debian.org CNAME=wilder.debian.org. A=*", false, 1, 1, "NOERROR"},
		{"invalid.com SERVFAIL", false, 0, 0, "SERVFAIL"},
		{"bad-line-without-domain", true, 0, 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			a, err := DNSAnswerFromString(tt.line)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DNSAnswerFromString(%q) error=%v, wantErr=%v", tt.line, err, tt.wantErr)
			}
			if err == nil {
				if len(a.A) != tt.wantA {
					t.Errorf("A record count mismatch: got=%d, want=%d", len(a.A), tt.wantA)
				}
				if len(a.CNAME) != tt.wantCNAME {
					t.Errorf("CNAME count mismatch: got=%d, want=%d", len(a.CNAME), tt.wantCNAME)
				}
				if !strings.EqualFold(a.Status, tt.wantStatus) {
					t.Errorf("Status mismatch: got=%s, want=%s", a.Status, tt.wantStatus)
				}
			}
		})
	}
}

// TestMatchRecords tests the matchRecords function with various scenarios
func TestMatchRecords(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		values   []string
		want     bool
	}{
		// Basic cases
		{
			name:     "exact match",
			patterns: []string{"1.2.3.4", "5.6.7.8"},
			values:   []string{"1.2.3.4", "5.6.7.8"},
			want:     true,
		},
		{
			name:     "different lengths",
			patterns: []string{"1.2.3.4"},
			values:   []string{"1.2.3.4", "5.6.7.8"},
			want:     false,
		},
		{
			name:     "empty slices",
			patterns: []string{},
			values:   []string{},
			want:     true,
		},

		// Wildcard cases
		{
			name:     "simple wildcard",
			patterns: []string{"192.168.*.*", "10.0.*.1"},
			values:   []string{"192.168.1.1", "10.0.2.1"},
			want:     true,
		},
		{
			name:     "wildcard with exact match",
			patterns: []string{"192.168.*.*", "10.0.0.1"},
			values:   []string{"192.168.1.1", "10.0.0.1"},
			want:     true,
		},

		// Ambiguous cases that require permutation
		{
			name:     "ambiguous wildcards",
			patterns: []string{"192.168.*.1", "192.*.1.1"},
			values:   []string{"192.168.1.1", "192.169.1.1"},
			want:     true,
		},
		{
			name:     "multiple ambiguous patterns",
			patterns: []string{"192.*.1.*", "192.168.*.*", "192.*.*.1"},
			values:   []string{"192.168.1.1", "192.169.2.1", "192.170.1.1"},
			want:     true,
		},

		// Domain cases
		{
			name:     "domain wildcards",
			patterns: []string{"*.example.com", "test.example.org"},
			values:   []string{"sub.example.com", "test.example.org"},
			want:     true,
		},
		{
			name:     "complex domain patterns",
			patterns: []string{"*.sub.*.com", "sub.*.example.*"},
			values:   []string{"sub.domain.example.com", "other.sub.test.com"},
			want:     true,
		},

		// Failure cases
		{
			name:     "no match possible",
			patterns: []string{"192.168.1.*", "192.*.1.*"},
			values:   []string{"192.168.2.1", "192.169.2.1"},
			want:     false,
		},
		{
			name:     "partial matches only",
			patterns: []string{"192.168.1.*", "192.*.1.*"},
			values:   []string{"192.168.2.1", "192.169.2.1"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchRecords(tt.patterns, tt.values)
			if got != tt.want {
				t.Errorf("matchRecords(%v, %v) = %v, want %v", tt.patterns, tt.values, got, tt.want)
			}
		})
	}
}

// TestNextPermutation tests the nextPermutation function
func TestNextPermutation(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		want     bool
		expected []int
	}{
		{
			name:     "simple permutation",
			input:    []int{0, 1, 2},
			want:     true,
			expected: []int{0, 2, 1},
		},
		{
			name:     "last permutation",
			input:    []int{2, 1, 0},
			want:     false,
			expected: []int{2, 1, 0},
		},
		{
			name:     "single element",
			input:    []int{0},
			want:     false,
			expected: []int{0},
		},
		{
			name:     "empty slice",
			input:    []int{},
			want:     false,
			expected: []int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextPermutation(tt.input)
			if got != tt.want {
				t.Errorf("nextPermutation(%v) = %v, want %v", tt.input, got, tt.want)
			}
			if !reflect.DeepEqual(tt.input, tt.expected) {
				t.Errorf("after nextPermutation, got %v, want %v", tt.input, tt.expected)
			}
		})
	}
}
