package cidr

import (
	"strings"
	"testing"
)

func TestParseCIDR(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "10.0.0.0/16", want: "10.0.0.0/16"},
		{in: "10.0.1.0/24", want: "10.0.1.0/24"},
		{in: "0.0.0.0/0", want: "0.0.0.0/0"},
		{in: "2001:db8::/32", want: "2001:db8::/32"},
		{in: "10.1.2.3/16", want: "10.1.0.0/16"},
		{in: "10.0.0.0", wantErr: true},
		{in: "not-a-cidr", wantErr: true},
		{in: "10.0.0.0/33", wantErr: true},
		{in: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseCIDR(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseCIDR(%q) expected error, got %s", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCIDR(%q): %v", tt.in, err)
			}
			if got.String() != tt.want {
				t.Errorf("ParseCIDR(%q) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsValid(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{in: "10.0.0.0/16", want: true},
		{in: "192.168.0.0/24", want: true},
		{in: "10.0.0.0", want: false},
		{in: "banana", want: false},
		{in: "10.0.0.0/64", want: false},
	}

	for _, tt := range tests {
		if got := IsValid(tt.in); got != tt.want {
			t.Errorf("IsValid(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		ip      string
		want    bool
		wantErr bool
	}{
		{name: "in vpc", cidr: "10.0.0.0/16", ip: "10.0.1.5", want: true},
		{name: "subnet boundary", cidr: "10.0.1.0/24", ip: "10.0.1.0", want: true},
		{name: "subnet broadcast", cidr: "10.0.1.0/24", ip: "10.0.1.255", want: true},
		{name: "outside vpc", cidr: "10.0.0.0/16", ip: "11.0.0.1", want: false},
		{name: "outside subnet", cidr: "10.0.1.0/24", ip: "10.0.2.10", want: false},
		{name: "default route matches everything", cidr: "0.0.0.0/0", ip: "203.0.113.7", want: true},
		{name: "ipv4-mapped in ipv4 prefix", cidr: "10.0.0.0/16", ip: "::ffff:10.0.2.3", want: true},
		{name: "ipv6 against ipv4 prefix", cidr: "10.0.0.0/16", ip: "2001:db8::1", want: false},
		{name: "invalid ip", cidr: "10.0.0.0/16", ip: "not-an-ip", wantErr: true},
		{name: "invalid cidr", cidr: "10.0.0.0", ip: "10.0.0.1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Contains(tt.cidr, tt.ip)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Contains(%q, %q) expected error, got %v", tt.cidr, tt.ip, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Contains(%q, %q): %v", tt.cidr, tt.ip, err)
			}
			if got != tt.want {
				t.Errorf("Contains(%q, %q) = %v, want %v", tt.cidr, tt.ip, got, tt.want)
			}
		})
	}
}

func TestContainsErrorMessages(t *testing.T) {
	if _, err := Contains("broken", "10.0.0.1"); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Errorf("expected cidr error mentioning the input, got %v", err)
	}
	if _, err := Contains("10.0.0.0/16", "nope"); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("expected ip error mentioning the input, got %v", err)
	}
}
