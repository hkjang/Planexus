package secure

import (
	"bytes"
	"testing"
)

func TestVaultRoundTripAndContextBinding(t *testing.T) {
	vault, err := NewVault(bytes.Repeat([]byte{0x2a}, 32))
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := vault.Encrypt([]byte("client-secret"), "authentication/oidc")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := vault.Decrypt(envelope, "authentication/oidc")
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != "client-secret" {
		t.Fatalf("got %q", plain)
	}
	if _, err := vault.Decrypt(envelope, "ai/provider"); err == nil {
		t.Fatal("expected context mismatch to reject decryption")
	}
}

func TestVaultUsesFreshNonce(t *testing.T) {
	vault, _ := NewVault(bytes.Repeat([]byte{0x42}, 32))
	a, _ := vault.Encrypt([]byte("same"), "context")
	b, _ := vault.Encrypt([]byte("same"), "context")
	if a == b {
		t.Fatal("envelopes must differ")
	}
}

func TestVaultMACBindsPayloadAndContext(t *testing.T) {
	vault, _ := NewVault(bytes.Repeat([]byte{0x7a}, 32))
	payload := []byte(`{"strategies.ndjson":"abc"}`)
	signature := vault.MAC(payload, "backup-integrity-v2")
	if !vault.VerifyMAC(payload, "backup-integrity-v2", signature) {
		t.Fatal("expected valid MAC")
	}
	if vault.VerifyMAC([]byte(`{"strategies.ndjson":"changed"}`), "backup-integrity-v2", signature) {
		t.Fatal("modified payload must fail verification")
	}
	if vault.VerifyMAC(payload, "other-context", signature) {
		t.Fatal("different context must fail verification")
	}
}
