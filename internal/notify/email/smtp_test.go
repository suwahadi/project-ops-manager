package email

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"net"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/notify"
)

type fakeSMTP struct {
	mu       sync.Mutex
	authUser string
	authPass string
	mailFrom string
	rcpt     string
	data     string
	usedTLS  bool
}

func serveSMTP(t *testing.T, cert tls.Certificate, f *fakeSMTP) (string, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.handle(c, cert)
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port
}

func (f *fakeSMTP) handle(c net.Conn, cert tls.Certificate) {
	defer c.Close()
	r := bufio.NewReader(c)
	w := func(s string) { _, _ = c.Write([]byte(s + "\r\n")) }
	w("220 fake ESMTP")
	tlsOn := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.TrimSpace(line)
		up := strings.ToUpper(cmd)
		switch {
		case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
			if tlsOn {
				w("250-fake")
				w("250 AUTH PLAIN LOGIN")
			} else {
				w("250-fake")
				w("250 STARTTLS")
			}
		case up == "STARTTLS":
			w("220 ready")
			tc := tls.Server(c, &tls.Config{Certificates: []tls.Certificate{cert}})
			if err := tc.Handshake(); err != nil {
				return
			}
			c = tc
			r = bufio.NewReader(tc)
			tlsOn = true
			f.mu.Lock()
			f.usedTLS = true
			f.mu.Unlock()
		case strings.HasPrefix(up, "AUTH PLAIN"):
			arg := strings.TrimSpace(cmd[len("AUTH PLAIN"):])
			if arg == "" {
				w("334 ")
				l, _ := r.ReadString('\n')
				arg = strings.TrimSpace(l)
			}
			raw, _ := base64.StdEncoding.DecodeString(arg)
			parts := strings.Split(string(raw), "\x00")
			if len(parts) == 3 {
				f.mu.Lock()
				f.authUser, f.authPass = parts[1], parts[2]
				f.mu.Unlock()
			}
			w("235 ok")
		case strings.HasPrefix(up, "MAIL FROM:"):
			f.mu.Lock()
			f.mailFrom = cmd[len("MAIL FROM:"):]
			f.mu.Unlock()
			w("250 ok")
		case strings.HasPrefix(up, "RCPT TO:"):
			f.mu.Lock()
			f.rcpt = cmd[len("RCPT TO:"):]
			f.mu.Unlock()
			w("250 ok")
		case up == "DATA":
			w("354 go")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil || l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			f.mu.Lock()
			f.data = b.String()
			f.mu.Unlock()
			w("250 queued")
		case up == "QUIT":
			w("221 bye")
			return
		default:
			w("250 ok")
		}
	}
}

func TestSendOverSTARTTLSWithAuthAndFromName(t *testing.T) {
	srv := httptest.NewTLSServer(nil)
	defer srv.Close()
	cert := srv.TLS.Certificates[0]
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())

	fake := &fakeSMTP{}
	host, port := serveSMTP(t, cert, fake)
	s := &Sender{Timeout: 5 * time.Second, TLSConfig: &tls.Config{RootCAs: pool, ServerName: "example.com"}}
	cfg := Config{Host: host, Port: port, Username: "relay-login@example.net", Password: "smtp-key-123", From: "ops@example.com", FromName: "Ops Manager", To: "me@example.com"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.Send(ctx, cfg, notify.Message{Subject: "Renewal in 7 days: example.com (Domain)", Body: "Hello"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if !fake.usedTLS {
		t.Fatal("STARTTLS was not used")
	}
	if fake.authUser != "relay-login@example.net" || fake.authPass != "smtp-key-123" {
		t.Fatalf("auth = %q / %q", fake.authUser, fake.authPass)
	}
	if !strings.Contains(fake.mailFrom, "<ops@example.com>") || !strings.Contains(fake.rcpt, "<me@example.com>") {
		t.Fatalf("envelope from %q rcpt %q", fake.mailFrom, fake.rcpt)
	}
	if !strings.Contains(fake.data, `From: "Ops Manager" <ops@example.com>`) || !strings.Contains(fake.data, "Subject: Renewal in 7 days") {
		t.Fatalf("message headers:\n%s", fake.data)
	}
}

func TestSendRefusesServerWithoutSTARTTLS(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		_, _ = c.Write([]byte("220 plain\r\n"))
		for {
			l, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if strings.HasPrefix(strings.ToUpper(l), "QUIT") {
				_, _ = c.Write([]byte("221 bye\r\n"))
				return
			}
			_, _ = c.Write([]byte("250 ok\r\n"))
		}
	}()
	port, _ := strconv.Atoi(strings.Split(ln.Addr().String(), ":")[1])
	s := &Sender{Timeout: 3 * time.Second}
	err = s.Send(context.Background(), Config{Host: "127.0.0.1", Port: port, Username: "u", Password: "secret-pass", From: "a@b.c", To: "d@e.f"}, notify.Message{Subject: "x", Body: "y"})
	if err == nil {
		t.Fatal("plaintext SMTP must be refused")
	}
	if strings.Contains(err.Error(), "secret-pass") {
		t.Fatal("error leaks the password")
	}
}
