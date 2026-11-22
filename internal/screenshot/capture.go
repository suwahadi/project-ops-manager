package screenshot

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/remote"
)

const (
	ViewportWidth  = 1440
	ViewportHeight = 900
	Quality        = 80
	Timeout        = 30 * time.Second
	settleDelay    = 1500 * time.Millisecond
)

type Capturer struct {
	ChromiumURL string
	Guard       *Guard
}

func NewCapturer(chromiumURL string, guard *Guard) *Capturer {
	return &Capturer{ChromiumURL: chromiumURL, Guard: guard}
}

func (c *Capturer) endpoint(ctx context.Context) (string, error) {
	u, err := url.Parse(c.ChromiumURL)
	if err != nil || u.Host == "" {
		return "", errors.New("OPS_CHROMIUM_URL is not a valid URL")
	}
	host := u.Hostname()
	if net.ParseIP(host) == nil && host != "localhost" {
		addrs, err := net.DefaultResolver.LookupHost(ctx, host)
		if err != nil || len(addrs) == 0 {
			return "", fmt.Errorf("the Chromium service %s is not reachable", host)
		}
		host = addrs[0]
	}
	port := u.Port()
	if port == "" {
		port = "9222"
	}
	u.Host = net.JoinHostPort(host, port)
	return u.String(), nil
}

func (c *Capturer) Capture(parent context.Context, target string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, Timeout)
	defer cancel()
	if err := c.Guard.CheckURL(ctx, target); err != nil {
		return nil, err
	}
	endpoint, err := c.endpoint(ctx)
	if err != nil {
		return nil, err
	}
	allocCtx, cancelAlloc := remote.NewAllocator(ctx, endpoint)
	defer cancelAlloc()
	tabCtx, cancelTab := chromedp.NewContext(allocCtx)
	defer cancelTab()

	events := chromedp.Events(tabCtx, fetch.RequestPaused)
	go func() {
		for ev, err := range events {
			if err != nil {
				return
			}
			go c.decide(tabCtx, ev)
		}
	}()
	if _, err := chromedp.Call(tabCtx, fetch.Enable, fetch.EnableParams{}); err != nil {
		return nil, friendlyError(ctx, err)
	}
	if _, err := chromedp.Call(tabCtx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{
		Width: ViewportWidth, Height: ViewportHeight, DeviceScaleFactor: 1,
	}); err != nil {
		return nil, friendlyError(ctx, err)
	}
	if err := chromedp.Do(tabCtx, chromedp.Navigate(target), chromedp.Sleep(settleDelay)); err != nil {
		return nil, friendlyError(ctx, err)
	}
	quality := int64(Quality)
	fromSurface := true
	res, err := chromedp.Call(tabCtx, page.CaptureScreenshot, page.CaptureScreenshotParams{
		Format: page.CaptureScreenshotFormatWebp, Quality: &quality, FromSurface: &fromSurface,
	})
	if err != nil {
		return nil, friendlyError(ctx, err)
	}
	buf := res.Data
	if len(buf) == 0 {
		return nil, errors.New("the browser returned an empty screenshot")
	}
	return buf, nil
}

func (c *Capturer) decide(ctx context.Context, ev fetch.EventRequestPaused) {
	if ev.Request != nil && c.Guard.AllowRequest(ctx, ev.Request.URL) {
		_, _ = chromedp.Call(ctx, fetch.ContinueRequest, fetch.ContinueRequestParams{RequestID: ev.RequestID})
		return
	}
	_, _ = chromedp.Call(ctx, fetch.FailRequest, fetch.FailRequestParams{RequestID: ev.RequestID, ErrorReason: network.ErrorReasonBlockedByClient})
}

func friendlyError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return errors.New("the page did not finish loading within 30 seconds")
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "ERR_BLOCKED_BY_CLIENT"):
		return ErrPrivateHost
	case strings.Contains(msg, "ERR_NAME_NOT_RESOLVED"):
		return errors.New("the host name could not be resolved")
	case strings.Contains(msg, "ERR_CONNECTION_REFUSED"), strings.Contains(msg, "ERR_CONNECTION_TIMED_OUT"):
		return errors.New("the website refused or timed out the connection")
	case strings.Contains(msg, "ERR_CERT"):
		return errors.New("the website has an invalid TLS certificate")
	case strings.Contains(msg, "connection refused"), strings.Contains(msg, "websocket"), strings.Contains(msg, "json/version"):
		return errors.New("the Chromium service is not reachable")
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return fmt.Errorf("capture failed: %s", msg)
}
