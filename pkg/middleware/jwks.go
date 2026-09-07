package middleware

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const jwksCacheTTL = 10 * time.Minute

// TokenVerifier validates a Supabase access token and returns trusted claims.
type TokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (AuthenticatedUser, error)
}

// JWKSVerifier verifies asymmetric Supabase JWTs locally and refreshes rotated
// signing keys from the project's JWKS endpoint.
type JWKSVerifier struct {
	issuer     string
	jwksURL    string
	httpClient *http.Client
	mu         sync.Mutex
	keys       map[string]interface{}
	refreshAt  time.Time
}

func NewJWKSVerifier(supabaseURL string) *JWKSVerifier {
	baseURL := strings.TrimRight(supabaseURL, "/")
	return &JWKSVerifier{
		issuer:     baseURL + "/auth/v1",
		jwksURL:    baseURL + "/auth/v1/.well-known/jwks.json",
		httpClient: &http.Client{Timeout: 5 * time.Second},
		keys:       make(map[string]interface{}),
	}
}

func (v *JWKSVerifier) Verify(ctx context.Context, rawToken string) (AuthenticatedUser, error) {
	parsed, err := jwt.Parse(rawToken, func(token *jwt.Token) (interface{}, error) {
		kid, ok := token.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, errors.New("JWT kid header is required")
		}
		return v.signingKey(ctx, kid)
	},
		jwt.WithValidMethods([]string{"RS256", "ES256"}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience("authenticated"),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !parsed.Valid {
		return AuthenticatedUser{}, fmt.Errorf("invalid Supabase access token: %w", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return AuthenticatedUser{}, errors.New("invalid Supabase token claims")
	}
	subject, err := claims.GetSubject()
	if err != nil || subject == "" {
		return AuthenticatedUser{}, errors.New("Supabase token subject is required")
	}

	user := AuthenticatedUser{ID: subject}
	user.Email, _ = claims["email"].(string)
	if metadata, ok := claims["user_metadata"].(map[string]interface{}); ok {
		user.Name, _ = metadata["full_name"].(string)
		if user.Name == "" {
			user.Name, _ = metadata["name"].(string)
		}
	}
	return user, nil
}

func (v *JWKSVerifier) signingKey(ctx context.Context, kid string) (interface{}, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if key, ok := v.keys[kid]; ok && time.Now().Before(v.refreshAt) {
		return key, nil
	}
	if err := v.refresh(ctx); err != nil {
		return nil, err
	}
	key, ok := v.keys[kid]
	if !ok {
		return nil, fmt.Errorf("signing key %q not found", kid)
	}
	return key, nil
}

func (v *JWKSVerifier) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}
	response, err := v.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch Supabase JWKS: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch Supabase JWKS: status %d", response.StatusCode)
	}

	var set struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&set); err != nil {
		return fmt.Errorf("decode Supabase JWKS: %w", err)
	}
	keys := make(map[string]interface{}, len(set.Keys))
	for _, item := range set.Keys {
		key, err := item.publicKey()
		if err == nil && item.Kid != "" {
			keys[item.Kid] = key
		}
	}
	if len(keys) == 0 {
		return errors.New("Supabase JWKS contains no supported signing keys")
	}
	v.keys = keys
	v.refreshAt = time.Now().Add(jwksCacheTTL)
	return nil
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func (key jwk) publicKey() (interface{}, error) {
	switch key.Kty {
	case "RSA":
		modulus, err := decodeBase64URLInteger(key.N)
		if err != nil {
			return nil, err
		}
		exponentValue, err := decodeBase64URLInteger(key.E)
		if err != nil || !exponentValue.IsInt64() {
			return nil, errors.New("invalid RSA exponent")
		}
		return &rsa.PublicKey{N: modulus, E: int(exponentValue.Int64())}, nil
	case "EC":
		curve := map[string]elliptic.Curve{
			"P-256": elliptic.P256(),
			"P-384": elliptic.P384(),
			"P-521": elliptic.P521(),
		}[key.Crv]
		if curve == nil {
			return nil, fmt.Errorf("unsupported EC curve %q", key.Crv)
		}
		x, err := decodeBase64URLInteger(key.X)
		if err != nil {
			return nil, err
		}
		y, err := decodeBase64URLInteger(key.Y)
		if err != nil || !curve.IsOnCurve(x, y) {
			return nil, errors.New("invalid EC public key")
		}
		return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
	default:
		return nil, fmt.Errorf("unsupported JWK type %q", key.Kty)
	}
}

func decodeBase64URLInteger(value string) (*big.Int, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) == 0 {
		return nil, errors.New("invalid base64url integer")
	}
	return new(big.Int).SetBytes(decoded), nil
}
