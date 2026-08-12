package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

const envelopeVersion = "v1"

type Vault struct {
	aead        cipher.AEAD
	fingerprint string
	macKey      [sha256.Size]byte
}

func NewVault(key []byte) (*Vault, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(key)
	deriver := hmac.New(sha256.New, key)
	_, _ = deriver.Write([]byte("planexus-vault-mac-v1"))
	var macKey [sha256.Size]byte
	copy(macKey[:], deriver.Sum(nil))
	return &Vault{aead: aead, fingerprint: base64.RawURLEncoding.EncodeToString(digest[:12]), macKey: macKey}, nil
}

func (v *Vault) Fingerprint() string { return v.fingerprint }

func (v *Vault) MAC(payload []byte, context string) string {
	mac := hmac.New(sha256.New, v.macKey[:])
	_, _ = mac.Write([]byte(context))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (v *Vault) VerifyMAC(payload []byte, context, encoded string) bool {
	expected, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, v.macKey[:])
	_, _ = mac.Write([]byte(context))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(payload)
	return hmac.Equal(expected, mac.Sum(nil))
}

func (v *Vault) Encrypt(plaintext []byte, context string) (string, error) {
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := v.aead.Seal(nil, nonce, plaintext, []byte(context))
	payload := append(nonce, sealed...)
	return envelopeVersion + ":" + base64.RawURLEncoding.EncodeToString(payload), nil
}

func (v *Vault) Decrypt(envelope, context string) ([]byte, error) {
	prefix := envelopeVersion + ":"
	if len(envelope) <= len(prefix) || envelope[:len(prefix)] != prefix {
		return nil, errors.New("unsupported encrypted envelope")
	}
	payload, err := base64.RawURLEncoding.DecodeString(envelope[len(prefix):])
	if err != nil {
		return nil, fmt.Errorf("decode encrypted envelope: %w", err)
	}
	if len(payload) < v.aead.NonceSize() {
		return nil, errors.New("invalid encrypted envelope")
	}
	return v.aead.Open(nil, payload[:v.aead.NonceSize()], payload[v.aead.NonceSize():], []byte(context))
}
