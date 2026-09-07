package scraper

import (
	"net"
	"testing"
)

func TestValidateWebsiteURLRejectsSSRFAndUnsafeURLs(t *testing.T) {
	invalid := []string{
		"file:///etc/passwd",
		"http://localhost/admin",
		"http://service.localhost/admin",
		"http://127.0.0.1/admin",
		"http://169.254.169.254/latest/meta-data",
		"http://10.0.0.5/internal",
		"http://100.64.0.1/internal",
		"http://user:password@example.com",
		"https://example.com:8443/admin",
	}
	for _, value := range invalid {
		if _, err := validateWebsiteURL(value); err == nil {
			t.Errorf("expected %q to be rejected", value)
		}
	}
	if _, err := validateWebsiteURL("https://example.com/knowledge"); err != nil {
		t.Fatalf("expected public HTTPS URL to be accepted: %v", err)
	}
	normalized, err := NormalizeWebsiteURL(" example.com/knowledge#section ")
	if err != nil || normalized != "https://example.com/knowledge" {
		t.Fatalf("unexpected normalized URL %q error=%v", normalized, err)
	}
}

func TestIsPublicIPRejectsReservedNetworks(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "10.0.0.1", "100.64.0.1", "192.0.2.1", "2001:db8::1"} {
		if isPublicIP(net.ParseIP(value)) {
			t.Errorf("expected %s to be non-public", value)
		}
	}
	if !isPublicIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("expected global unicast address to be public")
	}
}
