package certificate

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

func TestNewManualDNSProvider(t *testing.T) {
	p := NewManualDNSProvider()
	if p == nil {
		t.Fatal("expected non-nil ManualDNSProvider")
	}

	info := p.GetChallenge()
	if info != nil {
		t.Error("expected nil challenge before Present")
	}
}

func TestManualDNSProvider_Present(t *testing.T) {
	p := NewManualDNSProvider()
	ctx := context.Background()

	if err := p.Present(ctx, "example.com", "tok123", "auth123"); err != nil {
		t.Fatalf("Present: %v", err)
	}

	info := p.GetChallenge()
	if info == nil {
		t.Fatal("expected non-nil challenge after Present")
	}
	if info.Domain != "example.com" {
		t.Errorf("Domain = %q, want %q", info.Domain, "example.com")
	}
	if len(info.Records) == 0 {
		t.Fatal("expected at least one TXT record")
	}
	if len(info.Records[0].Value) == 0 {
		t.Error("expected non-empty TXT value")
	}
}

func TestManualDNSProvider_PresentMultiple(t *testing.T) {
	p := NewManualDNSProvider()
	ctx := context.Background()

	if err := p.Present(ctx, "example.com", "tok1", "auth1"); err != nil {
		t.Fatalf("Present: %v", err)
	}
	if err := p.Present(ctx, "example.com", "tok2", "auth2"); err != nil {
		t.Fatalf("Present: %v", err)
	}

	info := p.GetChallenge()
	if info == nil {
		t.Fatal("expected non-nil challenge")
	}
	if len(info.Records) != 2 {
		t.Errorf("expected 2 records, got %d", len(info.Records))
	}
}

func TestManualDNSProvider_CleanUp(t *testing.T) {
	p := NewManualDNSProvider()
	ctx := context.Background()

	// 为 nil 时为空操作
	if err := p.CleanUp(ctx, "example.com", "tok", "auth"); err != nil {
		t.Fatalf("CleanUp: %v", err)
	}

	// 先 Present 再 CleanUp
	if err := p.Present(ctx, "example.com", "tok", "auth"); err != nil {
		t.Fatalf("Present: %v", err)
	}
	if err := p.CleanUp(ctx, "example.com", "tok", "auth"); err != nil {
		t.Fatalf("CleanUp: %v", err)
	}
	if info := p.GetChallenge(); info != nil {
		t.Error("expected nil after CleanUp")
	}
}

func TestManualDNSProvider_GetChallengeReturnsCopy(t *testing.T) {
	p := NewManualDNSProvider()
	ctx := context.Background()

	if err := p.Present(ctx, "example.com", "tok", "auth"); err != nil {
		t.Fatalf("Present: %v", err)
	}

	info1 := p.GetChallenge()
	info2 := p.GetChallenge()

	// 修改副本不应影响原始数据
	info1.Records[0].Name = "mutated.example.com"
	if info2.Records[0].Name == "mutated.example.com" {
		t.Error("GetChallenge should return a copy, not a reference")
	}
}

func TestLoadPrivateKeyFromContent_Valid(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalECPrivateKey: %v", err)
	}

	pemBlock := &pem.Block{Type: "EC PRIVATE KEY", Bytes: der}
	pemBytes := pem.EncodeToMemory(pemBlock)

	loaded, err := loadPrivateKeyFromContent(pemBytes)
	if err != nil {
		t.Fatalf("loadPrivateKeyFromContent: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil key")
	}
}

func TestLoadPrivateKeyFromContent_InvalidPEM(t *testing.T) {
	_, err := loadPrivateKeyFromContent([]byte("not-a-pem"))
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
}

func TestLoadPrivateKeyFromContent_Empty(t *testing.T) {
	_, err := loadPrivateKeyFromContent([]byte{})
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestCertificateRequest(t *testing.T) {
	dnsID := 42
	req := CertificateRequest{
		Domain:        "example.com",
		Sans:          []string{"www.example.com", "api.example.com"},
		CAID:          1,
		DNSProviderID: &dnsID,
		AutoRenew:     true,
		RenewalDays:   30,
		KeyType:       "EC256",
	}

	if req.Domain != "example.com" {
		t.Errorf("Domain = %q, want %q", req.Domain, "example.com")
	}
	if len(req.Sans) != 2 {
		t.Errorf("Sans length = %d, want 2", len(req.Sans))
	}
	if req.CAID != 1 {
		t.Errorf("CAID = %d, want 1", req.CAID)
	}
	if req.DNSProviderID == nil || *req.DNSProviderID != 42 {
		t.Error("expected DNSProviderID to be 42")
	}
	if !req.AutoRenew {
		t.Error("expected AutoRenew to be true")
	}
	if req.RenewalDays != 30 {
		t.Errorf("RenewalDays = %d, want 30", req.RenewalDays)
	}
	if req.KeyType != "EC256" {
		t.Errorf("KeyType = %q, want %q", req.KeyType, "EC256")
	}
}
