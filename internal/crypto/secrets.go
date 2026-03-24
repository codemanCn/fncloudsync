package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

type SecretManager struct {
	aead cipher.AEAD
}

func NewSecretManager(key string) (*SecretManager, error) {
	if len(key) != 32 {
		return nil, errors.New("secret key must be 32 bytes")
	}

	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &SecretManager{aead: aead}, nil
}

func (m *SecretManager) EncryptString(plaintext string) (string, error) {
	nonce := make([]byte, m.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := m.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (m *SecretManager) DecryptString(ciphertext string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	nonceSize := m.aead.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce := raw[:nonceSize]
	encrypted := raw[nonceSize:]
	plaintext, err := m.aead.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
