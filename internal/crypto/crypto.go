package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

const NonceSize = 12

var ErrUnknownKeyVersion = errors.New("unknown encryption key version")

type Sealed struct {
	Ciphertext []byte
	Nonce      []byte
	KeyVersion int32
}

type Box struct {
	keys    map[int32]cipher.AEAD
	current int32
}

func NewBox(key []byte, version int32) (*Box, error) {
	b := &Box{keys: map[int32]cipher.AEAD{}}
	if err := b.AddKey(key, version); err != nil {
		return nil, err
	}
	b.current = version
	return b, nil
}

func (b *Box) AddKey(key []byte, version int32) error {
	if len(key) != 32 {
		return fmt.Errorf("encryption key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("create gcm: %w", err)
	}
	b.keys[version] = aead
	return nil
}

func (b *Box) CurrentVersion() int32 { return b.current }

func (b *Box) Seal(plaintext []byte) (Sealed, error) {
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return Sealed{}, fmt.Errorf("read nonce: %w", err)
	}
	aead := b.keys[b.current]
	return Sealed{Ciphertext: aead.Seal(nil, nonce, plaintext, nil), Nonce: nonce, KeyVersion: b.current}, nil
}

func (b *Box) Open(s Sealed) ([]byte, error) {
	aead, ok := b.keys[s.KeyVersion]
	if !ok {
		return nil, ErrUnknownKeyVersion
	}
	if len(s.Nonce) != NonceSize {
		return nil, errors.New("invalid nonce size")
	}
	plain, err := aead.Open(nil, s.Nonce, s.Ciphertext, nil)
	if err != nil {
		return nil, errors.New("decrypt failed")
	}
	return plain, nil
}

func (b *Box) SealString(s string) (Sealed, error) { return b.Seal([]byte(s)) }

func (b *Box) OpenString(s Sealed) (string, error) {
	p, err := b.Open(s)
	return string(p), err
}
