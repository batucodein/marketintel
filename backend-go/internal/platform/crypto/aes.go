// Package crypto provides AES-256-GCM encryption used to protect OAuth
// tokens and other credentials at rest.
//
// The key is a 32-byte secret. In production, set TOKEN_ENCRYPTION_KEY to
// a base64-encoded 32-byte value. In development the config falls back to
// deriving one from SECRET_KEY via SHA-256 so local dev works out of the box.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

type Cipher struct {
	aead cipher.AEAD
}

// NewFromKey returns a Cipher from a raw 32-byte key.
func NewFromKey(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new GCM: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// NewFromKeyOrSecret prefers base64Key if non-empty; otherwise derives a
// 32-byte key from secretFallback via SHA-256. Intended for dev friendliness.
func NewFromKeyOrSecret(base64Key, secretFallback string) (*Cipher, error) {
	if base64Key != "" {
		raw, err := base64.StdEncoding.DecodeString(base64Key)
		if err != nil {
			return nil, fmt.Errorf("crypto: decode base64 key: %w", err)
		}
		return NewFromKey(raw)
	}
	if secretFallback == "" {
		return nil, errors.New("crypto: no TOKEN_ENCRYPTION_KEY and no fallback")
	}
	sum := sha256.Sum256([]byte(secretFallback))
	return NewFromKey(sum[:])
}

// Encrypt returns nonce || ciphertext, all base64-encoded.
// Safe to store as a single string in the DB.
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: nonce: %w", err)
	}
	ct := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// Decrypt reverses Encrypt.
func (c *Cipher) Decrypt(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(stored)
	if err != nil {
		return "", fmt.Errorf("crypto: decode: %w", err)
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("crypto: ciphertext too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	plaintext, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("crypto: open: %w", err)
	}
	return string(plaintext), nil
}
