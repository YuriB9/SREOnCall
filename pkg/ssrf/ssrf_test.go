package ssrf

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestValidateURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		wantErr error
	}{
		// Literal public IP — no DNS needed, must pass.
		{name: "public literal https", raw: "https://8.8.8.8/hooks/abc", wantErr: nil},
		{name: "empty", raw: "", wantErr: ErrNotHTTPS},
		{name: "no host", raw: "https:///hooks/abc", wantErr: ErrInvalidURL},
		{name: "http scheme rejected", raw: "http://example.com/hooks/abc", wantErr: ErrNotHTTPS},
		{name: "no scheme rejected", raw: "example.com/hooks/abc", wantErr: ErrNotHTTPS},
		// Blocked targets — resolve locally, no external DNS.
		{name: "cloud metadata", raw: "https://169.254.169.254/latest/meta-data/", wantErr: ErrBlockedAddress},
		{name: "loopback ipv4", raw: "https://127.0.0.1/hooks/x", wantErr: ErrBlockedAddress},
		{name: "loopback ipv6", raw: "https://[::1]/hooks/x", wantErr: ErrBlockedAddress},
		{name: "localhost name", raw: "https://localhost/hooks/x", wantErr: ErrBlockedAddress},
		{name: "rfc1918 10", raw: "https://10.0.0.5/hooks/x", wantErr: ErrBlockedAddress},
		{name: "rfc1918 192168", raw: "https://192.168.1.1/hooks/x", wantErr: ErrBlockedAddress},
		{name: "rfc1918 172", raw: "https://172.16.0.1/hooks/x", wantErr: ErrBlockedAddress},
		{name: "unspecified", raw: "https://0.0.0.0/hooks/x", wantErr: ErrBlockedAddress},
		{name: "ula ipv6", raw: "https://[fc00::1]/hooks/x", wantErr: ErrBlockedAddress},
		{name: "link-local ipv6", raw: "https://[fe80::1]/hooks/x", wantErr: ErrBlockedAddress},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateURL(context.Background(), tt.raw)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidateURL(%q) = %v, want nil", tt.raw, err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateURL(%q) = %v, want %v", tt.raw, err, tt.wantErr)
			}
		})
	}
}

func TestIsBlockedIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{name: "public v4", ip: "8.8.8.8", want: false},
		{name: "public v6", ip: "2001:4860:4860::8888", want: false},
		{name: "loopback v4", ip: "127.0.0.1", want: true},
		{name: "loopback v6", ip: "::1", want: true},
		{name: "metadata", ip: "169.254.169.254", want: true},
		{name: "private 10", ip: "10.1.2.3", want: true},
		{name: "private 172", ip: "172.16.5.5", want: true},
		{name: "private 192168", ip: "192.168.0.1", want: true},
		{name: "unspecified v4", ip: "0.0.0.0", want: true},
		{name: "unspecified v6", ip: "::", want: true},
		{name: "ula v6", ip: "fc00::1", want: true},
		{name: "link-local v6", ip: "fe80::1", want: true},
		{name: "multicast v4", ip: "224.0.0.1", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("bad test IP %q", tt.ip)
			}
			if got := isBlockedIP(ip); got != tt.want {
				t.Fatalf("isBlockedIP(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}
