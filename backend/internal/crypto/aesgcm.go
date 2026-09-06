package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrNotEncrypted 表示输入不是本包产生的密文（无 v1: 前缀），用于存量明文检测。
var ErrNotEncrypted = errors.New("value is not aes-gcm encrypted")

const cipherPrefix = "v1:"

// Encrypt 用 AES-256-GCM 加密 plaintext，返回 "v1:" + base64(nonce ‖ sealed)。
// key 必须为 32 字节。
func Encrypt(key []byte, plaintext string) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("invalid key length: %d", len(key))
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce failed: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return cipherPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt 逆转 Encrypt。对无 v1: 前缀的输入返回 ErrNotEncrypted（存量明文检测）。
func Decrypt(key []byte, ciphertext string) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("invalid key length: %d", len(key))
	}
	if !strings.HasPrefix(ciphertext, cipherPrefix) {
		return "", ErrNotEncrypted
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertext, cipherPrefix))
	if err != nil {
		return "", fmt.Errorf("decode ciphertext failed: %w", err)
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	pt, err := gcm.Open(nil, raw[:nonceSize], raw[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt failed: %w", err)
	}
	return string(pt), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("init cipher failed: %w", err)
	}
	return cipher.NewGCM(block)
}
