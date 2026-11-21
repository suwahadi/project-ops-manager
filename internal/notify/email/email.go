package email

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	mail "github.com/wneessen/go-mail"

	"github.com/suwahadi/project-ops-manager/internal/notify"
)

type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       string
}

type Sender struct {
	Timeout time.Duration
}

func NewSender() *Sender { return &Sender{Timeout: 30 * time.Second} }

func (c Config) Validate() error {
	var missing []string
	if strings.TrimSpace(c.Host) == "" {
		missing = append(missing, "host")
	}
	if c.Port <= 0 || c.Port > 65535 {
		missing = append(missing, "port")
	}
	if strings.TrimSpace(c.From) == "" {
		missing = append(missing, "from address")
	}
	if strings.TrimSpace(c.To) == "" {
		missing = append(missing, "to address")
	}
	if len(missing) > 0 {
		return fmt.Errorf("SMTP %s required", strings.Join(missing, ", "))
	}
	return nil
}

func BuildMessage(cfg Config, m notify.Message) (*mail.Msg, error) {
	msg := mail.NewMsg()
	if err := msg.From(cfg.From); err != nil {
		return nil, fmt.Errorf("invalid from address: %w", err)
	}
	if err := msg.To(cfg.To); err != nil {
		return nil, fmt.Errorf("invalid to address: %w", err)
	}
	msg.Subject(m.Subject)
	msg.SetBodyString(mail.TypeTextPlain, m.Body)
	msg.SetDate()
	msg.SetMessageID()
	return msg, nil
}

func (s *Sender) Send(ctx context.Context, cfg Config, m notify.Message) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	msg, err := BuildMessage(cfg, m)
	if err != nil {
		return err
	}
	opts := []mail.Option{mail.WithPort(cfg.Port), mail.WithTimeout(s.Timeout)}
	if cfg.Port == 465 {
		opts = append(opts, mail.WithSSL())
	} else {
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	}
	if cfg.Username != "" {
		opts = append(opts, mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover), mail.WithUsername(cfg.Username), mail.WithPassword(cfg.Password))
	}
	client, err := mail.NewClient(cfg.Host, opts...)
	if err != nil {
		return fmt.Errorf("create SMTP client: %w", err)
	}
	if err := client.DialAndSendWithContext(ctx, msg); err != nil {
		msg := err.Error()
		if cfg.Password != "" {
			msg = strings.ReplaceAll(msg, cfg.Password, "[redacted]")
		}
		return errors.New("send email: " + msg)
	}
	return nil
}
