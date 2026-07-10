package certificate

import (
	"context"
	"sync"
	"testing"
)

// TestConcurrentManualDNSProvider 并发 Present/CleanUp/GetChallenge
func TestConcurrentManualDNSProvider(t *testing.T) {
	p := NewManualDNSProvider()
	ctx := context.Background()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			domain := "example.com"

			// Present
			_ = p.Present(ctx, domain, "tok", "auth")

			// GetChallenge
			info := p.GetChallenge()
			if info != nil && info.Domain != domain {
				t.Errorf("GetChallenge().Domain = %q, want %q", info.Domain, domain)
			}

			// CleanUp
			_ = p.CleanUp(ctx, domain, "tok", "auth")
		}(i)
	}

	wg.Wait()
}

// TestConcurrentManualDNSMultipleDomains 并发多个域名
func TestConcurrentManualDNSMultipleDomains(t *testing.T) {
	p := NewManualDNSProvider()
	ctx := context.Background()

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			domain := "example" + string(rune('a'+idx)) + ".com"
			_ = p.Present(ctx, domain, "tok", "auth")
			_ = p.GetChallenge()
			_ = p.CleanUp(ctx, domain, "tok", "auth")
		}(i)
	}

	wg.Wait()
}
