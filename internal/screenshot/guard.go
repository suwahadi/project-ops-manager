package screenshot

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

var (
	ErrScheme      = errors.New("only http and https URLs can be captured")
	ErrPrivateHost = errors.New("the URL points to a private, loopback or link-local address")
)

var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func IsPublic(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() ||
		a.IsInterfaceLocalMulticast() || a.IsMulticast() || a.IsUnspecified() {
		return false
	}
	for _, p := range blockedPrefixes {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

type Guard struct {
	Resolver Resolver
}

func NewGuard() *Guard { return &Guard{Resolver: net.DefaultResolver} }

func (g *Guard) CheckURL(ctx context.Context, raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return errors.New("the URL is not valid")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrScheme
	}
	return g.CheckHost(ctx, u.Hostname())
}

func (g *Guard) CheckHost(ctx context.Context, host string) error {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "" {
		return errors.New("the URL has no host")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".local") {
		return ErrPrivateHost
	}
	if a, err := netip.ParseAddr(host); err == nil {
		if !IsPublic(a) {
			return ErrPrivateHost
		}
		return nil
	}
	addrs, err := g.Resolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addrs) == 0 {
		return fmt.Errorf("could not resolve %s", host)
	}
	for _, a := range addrs {
		if !IsPublic(a) {
			return ErrPrivateHost
		}
	}
	return nil
}

func (g *Guard) AllowRequest(ctx context.Context, raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch u.Scheme {
	case "data", "blob", "about":
		return true
	case "http", "https", "ws", "wss":
		return g.CheckHost(ctx, u.Hostname()) == nil
	}
	return false
}
