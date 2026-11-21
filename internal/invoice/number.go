package invoice

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
)

const (
	DefaultPrefix = "INV-DEV-"
	suffixLength  = 8
	alphabet      = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	MaxNumberLen  = 40
)

var numberPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._-]*$`)

func GenerateNumber(prefix string) (string, error) {
	var b strings.Builder
	b.WriteString(NormalizeNumber(prefix))
	limit := 256 - 256%len(alphabet)
	buf := make([]byte, 1)
	for n := 0; n < suffixLength; {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("generate invoice number: %w", err)
		}
		if int(buf[0]) >= limit {
			continue
		}
		b.WriteByte(alphabet[int(buf[0])%len(alphabet)])
		n++
	}
	return b.String(), nil
}

func NormalizeNumber(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

func ValidNumber(s string) bool {
	return len(s) <= MaxNumberLen && numberPattern.MatchString(s)
}

func ValidPrefix(s string) bool {
	return s == "" || (len(s) <= MaxNumberLen-suffixLength && numberPattern.MatchString(s))
}
