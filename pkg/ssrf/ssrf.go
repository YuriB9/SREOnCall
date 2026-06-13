// Package ssrf guards outbound HTTP against server-side request forgery by
// rejecting URLs and dial targets that resolve to non-public addresses
// (loopback, private, link-local, unspecified, multicast, unique-local).
package ssrf

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
)

var (
	// ErrInvalidURL means the value could not be parsed as a usable URL or its
	// host could not be resolved.
	ErrInvalidURL = errors.New("ssrf: invalid URL")
	// ErrNotHTTPS means the URL does not use the https scheme.
	ErrNotHTTPS = errors.New("ssrf: URL scheme must be https")
	// ErrBlockedAddress means the URL resolves to a non-public address.
	ErrBlockedAddress = errors.New("ssrf: address is not allowed")
)

// ValidateURL reports whether raw is a safe outbound webhook target: it must use
// https, and its host must resolve only to public addresses. A host that is (or
// resolves to) a loopback, private, link-local, unspecified, multicast or
// unique-local address is rejected with ErrBlockedAddress. DNS resolution honours
// ctx for cancellation and deadlines.
func ValidateURL(ctx context.Context, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidURL, err)
	}
	if u.Scheme != "https" {
		return ErrNotHTTPS
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%w: empty host", ErrInvalidURL)
	}

	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("%w: %s", ErrBlockedAddress, ip)
		}
		return nil
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("%w: resolve %q: %w", ErrInvalidURL, host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("%w: no addresses for %q", ErrInvalidURL, host)
	}
	for _, addr := range ips {
		if isBlockedIP(addr.IP) {
			return fmt.Errorf("%w: %s resolves to %s", ErrBlockedAddress, host, addr.IP)
		}
	}
	return nil
}

// GuardedDialContext wraps base's dialing so connections to blocked (non-public)
// addresses are refused at dial time. This is defense in depth on top of
// ValidateURL: it catches the TOCTOU window between validation and send, DNS
// rebinding, and HTTP redirects pointing at private addresses. It resolves the
// host once, rejects on any blocked IP, and dials only the validated addresses
// so the connection cannot land on a different IP than the one that was checked.
func GuardedDialContext(base *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	if base == nil {
		base = &net.Dialer{}
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("%w: no addresses for %q", ErrInvalidURL, host)
		}
		for _, addr := range ips {
			if isBlockedIP(addr.IP) {
				return nil, fmt.Errorf("%w: %s resolves to %s", ErrBlockedAddress, host, addr.IP)
			}
		}
		var lastErr error
		for _, addr := range ips {
			conn, err := base.DialContext(ctx, network, net.JoinHostPort(addr.IP.String(), port))
			if err != nil {
				lastErr = err
				continue
			}
			return conn, nil
		}
		return nil, lastErr
	}
}

// isBlockedIP reports whether ip must never be a target for tenant-controlled
// outbound requests. net.IP.IsPrivate covers RFC 1918 and the IPv6 unique-local
// range fc00::/7; IsLinkLocalUnicast covers 169.254.0.0/16 (incl. cloud metadata
// at 169.254.169.254) and fe80::/10; IsLoopback covers 127.0.0.0/8 and ::1.
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}
