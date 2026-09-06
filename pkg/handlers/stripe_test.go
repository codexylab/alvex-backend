package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestVerifyStripeSignature(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	payload := []byte(`{"id":"evt_test"}`)
	secret := "whsec_test"
	timestamp := fmt.Sprintf("%d", now.Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "." + string(payload)))
	header := "t=" + timestamp + ",v1=" + hex.EncodeToString(mac.Sum(nil))

	if !verifyStripeSignature(header, payload, secret, now, 5*time.Minute) {
		t.Fatal("expected valid signature to pass")
	}
	if verifyStripeSignature(header, []byte(`{"tampered":true}`), secret, now, 5*time.Minute) {
		t.Fatal("expected tampered payload to fail")
	}
	if verifyStripeSignature(header, payload, secret, now.Add(6*time.Minute), 5*time.Minute) {
		t.Fatal("expected stale signature to fail")
	}
	if verifyStripeSignature("", payload, secret, now, 5*time.Minute) {
		t.Fatal("expected missing signature to fail")
	}
}

func TestStripeHandlerRejectsMissingSignatureWithUnauthorized(t *testing.T) {
	handler := NewStripeHandler(nil, "whsec_test")
	request := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(`{"id":"evt_test"}`))
	recorder := httptest.NewRecorder()

	handler.HandleWebhook(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "Invalid Stripe webhook signature") {
		t.Fatalf("unexpected response %s", recorder.Body.String())
	}
}
