package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

type DecryptionResult struct {
	Plaintext       string
	UsedFallbackKey bool
}

// DeriveKey stretches the configuration key using HKDF-SHA256 to guarantee a secure 32-byte key.
func DeriveKey(configKey string) []byte {
	// Static salt and info to ensure deterministic key derivation for credentials.
	salt := []byte("paas-secrets-salt-v1")
	info := []byte("credential-encryption-info")
	hkdfReader := hkdf.New(sha256.New, []byte(configKey), salt, info)

	key := make([]byte, 32)
	_, _ = io.ReadFull(hkdfReader, key)
	return key
}

// DeriveKeyLegacy stretches the configuration key using SHA-256 (for backward-compatible fallback).
func DeriveKeyLegacy(configKey string) []byte {
	h := sha256.New()
	h.Write([]byte(configKey))
	return h.Sum(nil)
}

// Encrypt encrypts plaintext using AES-256-GCM, prepending the unique nonce to the ciphertext, encoded as base64.
func Encrypt(plaintext string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", errors.New("key must be exactly 32 bytes for AES-256")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// Seal appends ciphertext to nonce, making storage and transmission simple.
	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts the base64-encoded encrypted text using AES-256-GCM.
// It accepts one or more derived keys (e.g., for backward-compatible fallback).
// It attempts to decrypt using each key in order and returns the first successful decryption.
func Decrypt(encryptedB64 string, keys ...[]byte) (string, error) {
	result, err := DecryptWithResult(encryptedB64, keys...)
	if err != nil {
		return "", err
	}
	return result.Plaintext, nil
}

// DecryptWithResult decrypts with a keyring and reports whether a non-primary key succeeded.
func DecryptWithResult(encryptedB64 string, keys ...[]byte) (DecryptionResult, error) {
	if len(keys) == 0 {
		return DecryptionResult{}, errors.New("at least one key must be provided for decryption")
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encryptedB64)
	if err != nil {
		return DecryptionResult{}, err
	}

	var lastErr error
	for index, key := range keys {
		if len(key) != 32 {
			lastErr = errors.New("key must be exactly 32 bytes for AES-256")
			continue
		}

		block, err := aes.NewCipher(key)
		if err != nil {
			lastErr = err
			continue
		}

		aesGCM, err := cipher.NewGCM(block)
		if err != nil {
			lastErr = err
			continue
		}

		nonceSize := aesGCM.NonceSize()
		if len(ciphertext) < nonceSize {
			lastErr = errors.New("ciphertext too short")
			continue
		}

		nonce, actualCiphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
		plaintext, err := aesGCM.Open(nil, nonce, actualCiphertext, nil)
		if err != nil {
			lastErr = err
			continue
		}

		return DecryptionResult{
			Plaintext:       string(plaintext),
			UsedFallbackKey: index > 0,
		}, nil
	}

	if lastErr != nil {
		return DecryptionResult{}, fmt.Errorf("decryption failed with all provided keys; last error: %w", lastErr)
	}
	return DecryptionResult{}, errors.New("decryption failed")
}

// CredentialDecryptionKeys returns the current HKDF key, current legacy key, then previous keys.
func CredentialDecryptionKeys(current string, previous []string) [][]byte {
	keys := make([][]byte, 0, 2+(len(previous)*2))
	keys = append(keys, DeriveKey(current), DeriveKeyLegacy(current))

	seen := map[string]struct{}{current: {}}
	for _, secret := range previous {
		if secret == "" {
			continue
		}
		if _, ok := seen[secret]; ok {
			continue
		}
		seen[secret] = struct{}{}
		keys = append(keys, DeriveKey(secret), DeriveKeyLegacy(secret))
	}
	return keys
}
