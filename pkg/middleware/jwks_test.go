package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWKSVerifierValidatesClaimsAndCachesSigningKey(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/v1/.well-known/jwks.json" {
			http.NotFound(w, r)
			return
		}
		requests.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": []map[string]string{{
				"kty": "RSA",
				"kid": "test-key",
				"n":   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
				"e":   encodeJWKInteger(privateKey.PublicKey.E),
			}},
		})
	}))
	defer server.Close()

	claims := jwt.MapClaims{
		"iss":           server.URL + "/auth/v1",
		"aud":           "authenticated",
		"sub":           "user_123",
		"exp":           time.Now().Add(time.Hour).Unix(),
		"email":         "owner@example.com",
		"user_metadata": map[string]interface{}{"full_name": "Owner Example"},
	}
	rawToken := signedJWT(t, privateKey, claims)
	verifier := NewJWKSVerifier(server.URL)

	for attempt := 0; attempt < 2; attempt++ {
		user, err := verifier.Verify(context.Background(), rawToken)
		if err != nil {
			t.Fatalf("verify token: %v", err)
		}
		if user.ID != "user_123" || user.Email != "owner@example.com" || user.Name != "Owner Example" {
			t.Fatalf("unexpected verified user: %#v", user)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("expected one JWKS fetch, got %d", got)
	}
}

func TestJWKSVerifierRejectsWrongAudience(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": []map[string]string{{
				"kty": "RSA",
				"kid": "test-key",
				"n":   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
				"e":   encodeJWKInteger(privateKey.PublicKey.E),
			}},
		})
	}))
	defer server.Close()

	rawToken := signedJWT(t, privateKey, jwt.MapClaims{
		"iss": server.URL + "/auth/v1",
		"aud": "service_role",
		"sub": "user_123",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	if _, err := NewJWKSVerifier(server.URL).Verify(context.Background(), rawToken); err == nil {
		t.Fatal("expected wrong audience to be rejected")
	}
}

func signedJWT(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-key"
	rawToken, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return rawToken
}

func encodeJWKInteger(value int) string {
	return base64.RawURLEncoding.EncodeToString(big.NewInt(int64(value)).Bytes())
}
