package crypto

import (
	"bytes"
	"testing"
)

func key(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func TestSealOpenRoundTrip(t *testing.T) {
	box, err := NewBox(key(1), 1)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := box.SealString("s3cret-pässword")
	if err != nil {
		t.Fatal(err)
	}
	if len(sealed.Nonce) != NonceSize {
		t.Fatalf("nonce size = %d", len(sealed.Nonce))
	}
	if bytes.Contains(sealed.Ciphertext, []byte("s3cret")) {
		t.Fatal("ciphertext contains plaintext")
	}
	got, err := box.OpenString(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if got != "s3cret-pässword" {
		t.Fatalf("got %q", got)
	}
}

func TestSealUsesFreshNonce(t *testing.T) {
	box, _ := NewBox(key(1), 1)
	a, _ := box.SealString("x")
	b, _ := box.SealString("x")
	if bytes.Equal(a.Nonce, b.Nonce) || bytes.Equal(a.Ciphertext, b.Ciphertext) {
		t.Fatal("expected different nonce and ciphertext")
	}
}

func TestOpenRejectsTamperingAndWrongKey(t *testing.T) {
	box, _ := NewBox(key(1), 1)
	s, _ := box.SealString("value")
	tampered := s
	tampered.Ciphertext = append([]byte{}, s.Ciphertext...)
	tampered.Ciphertext[0] ^= 0xff
	if _, err := box.Open(tampered); err == nil {
		t.Fatal("expected tamper error")
	}
	other, _ := NewBox(key(2), 1)
	if _, err := other.Open(s); err == nil {
		t.Fatal("expected wrong key error")
	}
	unknown := s
	unknown.KeyVersion = 9
	if _, err := box.Open(unknown); err != ErrUnknownKeyVersion {
		t.Fatalf("expected unknown version error, got %v", err)
	}
}

func TestRotation(t *testing.T) {
	oldBox, _ := NewBox(key(1), 1)
	s, _ := oldBox.SealString("rotate me")
	newBox, _ := NewBox(key(2), 2)
	if err := newBox.AddKey(key(1), 1); err != nil {
		t.Fatal(err)
	}
	plain, err := newBox.OpenString(s)
	if err != nil || plain != "rotate me" {
		t.Fatalf("open old value: %q %v", plain, err)
	}
	re, _ := newBox.SealString(plain)
	if re.KeyVersion != 2 {
		t.Fatalf("version = %d", re.KeyVersion)
	}
}
