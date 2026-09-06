package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrIdentityInvitationNotConfigured = errors.New("identity invitation provider is not configured")

// InvitedIdentity is the minimum trusted result required from Supabase Auth.
type InvitedIdentity struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type IdentityInviter interface {
	InviteUser(ctx context.Context, email, name, redirectURL string) (*InvitedIdentity, error)
}

type SupabaseAdminClient struct {
	baseURL   string
	secretKey string
	http      *http.Client
}

func NewSupabaseAdminClient(baseURL, secretKey string) *SupabaseAdminClient {
	return &SupabaseAdminClient{
		baseURL:   strings.TrimRight(baseURL, "/"),
		secretKey: secretKey,
		http:      &http.Client{Timeout: 10 * time.Second},
	}
}

// InviteUser calls Supabase's server-only Auth Admin invite endpoint. The
// secret key is used only in headers and is never included in errors or logs.
func (c *SupabaseAdminClient) InviteUser(
	ctx context.Context,
	email string,
	name string,
	redirectURL string,
) (*InvitedIdentity, error) {
	if c.baseURL == "" || c.secretKey == "" {
		return nil, ErrIdentityInvitationNotConfigured
	}

	body, err := json.Marshal(map[string]interface{}{
		"email": email,
		"data":  map[string]string{"full_name": name},
	})
	if err != nil {
		return nil, fmt.Errorf("encode Supabase invitation: %w", err)
	}

	endpoint := c.baseURL + "/auth/v1/invite"
	if redirectURL != "" {
		endpoint += "?redirect_to=" + url.QueryEscape(redirectURL)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create Supabase invitation request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.secretKey)
	request.Header.Set("apikey", c.secretKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send Supabase invitation: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return nil, fmt.Errorf("Supabase invitation failed with status %d", response.StatusCode)
	}

	var identity InvitedIdentity
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&identity); err != nil {
		return nil, fmt.Errorf("decode Supabase invitation response: %w", err)
	}
	if identity.ID == "" || identity.Email == "" {
		return nil, fmt.Errorf("Supabase invitation response is missing user identity")
	}
	return &identity, nil
}
