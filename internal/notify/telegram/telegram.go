package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/notify"
)

const DefaultAPIBase = "https://api.telegram.org"

type Config struct {
	Token  string
	ChatID string
}

type Sender struct {
	APIBase string
	Client  *http.Client
}

func NewSender() *Sender {
	return &Sender{APIBase: DefaultAPIBase, Client: &http.Client{Timeout: 20 * time.Second}}
}

func (s *Sender) Send(ctx context.Context, cfg Config, msg notify.Message) error {
	if strings.TrimSpace(cfg.Token) == "" || strings.TrimSpace(cfg.ChatID) == "" {
		return errors.New("telegram bot token and chat ID are required")
	}
	text := msg.Subject + "\n\n" + msg.Body
	if len(text) > 4000 {
		text = text[:4000]
	}
	payload, err := json.Marshal(map[string]any{"chat_id": cfg.ChatID, "text": text, "disable_web_page_preview": true})
	if err != nil {
		return err
	}
	endpoint := s.APIBase + "/bot" + cfg.Token + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return errors.New("build telegram request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.Client.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			return fmt.Errorf("telegram request failed: %w", uerr.Err)
		}
		return errors.New("telegram request failed")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal(body, &result)
	if resp.StatusCode != http.StatusOK || !result.OK {
		desc := strings.ReplaceAll(result.Description, cfg.Token, "[redacted]")
		if desc == "" {
			desc = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("telegram API error %d: %s", resp.StatusCode, desc)
	}
	return nil
}
