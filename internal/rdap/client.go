package rdap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	DefaultBootstrapURL = "https://data.iana.org/rdap/dns.json"
	bootstrapTTL        = 24 * time.Hour
	requestTimeout      = 15 * time.Second
	minInterval         = time.Second
)

var (
	ErrUnsupported = errors.New("unsupported")
	ErrNotFound    = errors.New("domain not found at the registry")
)

type Client struct {
	BootstrapURL string
	HTTP         *http.Client
	Now          func() time.Time
	Sleep        func(context.Context, time.Duration) error
	Timeout      time.Duration

	mu        sync.Mutex
	services  map[string]string
	fetchedAt time.Time

	limitMu sync.Mutex
	nextAt  map[string]time.Time
}

func NewClient() *Client {
	return &Client{
		BootstrapURL: DefaultBootstrapURL,
		HTTP:         &http.Client{Timeout: requestTimeout},
		Now:          time.Now,
		Sleep:        sleepCtx,
		Timeout:      requestTimeout,
		nextAt:       map[string]time.Time{},
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

type bootstrapFile struct {
	Services [][][]string `json:"services"`
}

func ParseBootstrap(data []byte) (map[string]string, error) {
	var f bootstrapFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse bootstrap: %w", err)
	}
	out := map[string]string{}
	for _, svc := range f.Services {
		if len(svc) < 2 || len(svc[1]) == 0 {
			continue
		}
		base := ""
		for _, u := range svc[1] {
			if strings.HasPrefix(u, "https://") {
				base = u
				break
			}
		}
		if base == "" {
			base = svc[1][0]
		}
		if !strings.HasSuffix(base, "/") {
			base += "/"
		}
		for _, tld := range svc[0] {
			out[strings.ToLower(tld)] = base
		}
	}
	if len(out) == 0 {
		return nil, errors.New("bootstrap file has no services")
	}
	return out, nil
}

func (c *Client) bootstrap(ctx context.Context) (map[string]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.services != nil && c.Now().Sub(c.fetchedAt) < bootstrapTTL {
		return c.services, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BootstrapURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		if c.services != nil {
			return c.services, nil
		}
		return nil, fmt.Errorf("fetch IANA bootstrap: %w", unwrapURL(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if c.services != nil {
			return c.services, nil
		}
		return nil, fmt.Errorf("fetch IANA bootstrap: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read IANA bootstrap: %w", err)
	}
	services, err := ParseBootstrap(data)
	if err != nil {
		return nil, err
	}
	c.services, c.fetchedAt = services, c.Now()
	return services, nil
}

func unwrapURL(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return uerr.Err
	}
	return err
}

func NormalizeDomain(name string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(name))
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimPrefix(s, "www.")
	s = strings.Trim(s, ".")
	if s == "" || !strings.Contains(s, ".") {
		return "", errors.New("asset name is not a domain name")
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '.') {
			return "", errors.New("asset name is not an ASCII domain name")
		}
	}
	return s, nil
}

func serverFor(services map[string]string, domain string) (string, bool) {
	labels := strings.Split(domain, ".")
	for i := 1; i < len(labels); i++ {
		if base, ok := services[strings.Join(labels[i:], ".")]; ok {
			return base, true
		}
	}
	return "", false
}

func (c *Client) wait(ctx context.Context, host string) error {
	c.limitMu.Lock()
	now := c.Now()
	at := c.nextAt[host]
	if at.Before(now) {
		at = now
	}
	c.nextAt[host] = at.Add(minInterval)
	c.limitMu.Unlock()
	return c.Sleep(ctx, at.Sub(now))
}

type Response struct {
	Events []struct {
		Action string `json:"eventAction"`
		Date   string `json:"eventDate"`
	} `json:"events"`
}

func ParseExpiration(data []byte) (time.Time, error) {
	var r Response
	if err := json.Unmarshal(data, &r); err != nil {
		return time.Time{}, fmt.Errorf("parse RDAP response: %w", err)
	}
	for _, e := range r.Events {
		if strings.EqualFold(e.Action, "expiration") {
			for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
				if t, err := time.Parse(layout, e.Date); err == nil {
					u := t.UTC()
					return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC), nil
				}
			}
			return time.Time{}, fmt.Errorf("invalid expiration date %q", e.Date)
		}
	}
	return time.Time{}, ErrUnsupported
}

func (c *Client) Expiration(ctx context.Context, name string) (time.Time, error) {
	domain, err := NormalizeDomain(name)
	if err != nil {
		return time.Time{}, err
	}
	services, err := c.bootstrap(ctx)
	if err != nil {
		return time.Time{}, err
	}
	base, ok := serverFor(services, domain)
	if !ok {
		return time.Time{}, ErrUnsupported
	}
	u, err := url.Parse(base + "domain/" + domain)
	if err != nil {
		return time.Time{}, fmt.Errorf("build RDAP URL: %w", err)
	}
	if err := c.wait(ctx, u.Host); err != nil {
		return time.Time{}, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return time.Time{}, err
	}
	req.Header.Set("Accept", "application/rdap+json, application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return time.Time{}, fmt.Errorf("RDAP request to %s failed: %w", u.Host, unwrapURL(err))
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return time.Time{}, ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return time.Time{}, fmt.Errorf("RDAP server %s returned HTTP %d", u.Host, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return time.Time{}, fmt.Errorf("read RDAP response: %w", err)
	}
	return ParseExpiration(data)
}
