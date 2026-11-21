package screenshot

import (
	"context"
	"errors"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/remote"
)

const pdfTimeout = 30 * time.Second

func (c *Capturer) PrintPDF(parent context.Context, html string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, pdfTimeout)
	defer cancel()
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
			go func() {
				_, _ = chromedp.Call(tabCtx, fetch.FailRequest, fetch.FailRequestParams{RequestID: ev.RequestID, ErrorReason: network.ErrorReasonBlockedByClient})
			}()
		}
	}()
	if _, err := chromedp.Call(tabCtx, fetch.Enable, fetch.EnableParams{}); err != nil {
		return nil, pdfError(ctx, err)
	}
	if err := chromedp.Do(tabCtx, chromedp.Navigate("about:blank")); err != nil {
		return nil, pdfError(ctx, err)
	}
	tree, err := chromedp.Call(tabCtx, page.GetFrameTree, cdp.Empty{})
	if err != nil {
		return nil, pdfError(ctx, err)
	}
	if _, err := chromedp.Call(tabCtx, page.SetDocumentContent, page.SetDocumentContentParams{FrameID: tree.FrameTree.Frame.ID, HTML: html}); err != nil {
		return nil, pdfError(ctx, err)
	}
	yes := true
	res, err := chromedp.Call(tabCtx, page.PrintToPDF, page.PrintToPDFParams{
		PrintBackground:   &yes,
		PreferCSSPageSize: &yes,
		MarginTop:         new(float64),
		MarginBottom:      new(float64),
		MarginLeft:        new(float64),
		MarginRight:       new(float64),
	})
	if err != nil {
		return nil, pdfError(ctx, err)
	}
	if len(res.Data) == 0 {
		return nil, errors.New("the browser returned an empty PDF")
	}
	return res.Data, nil
}

func pdfError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return errors.New("the PDF was not ready within 30 seconds")
	}
	return friendlyError(ctx, err)
}
