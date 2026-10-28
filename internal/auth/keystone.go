package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/config"
)

// Keystone uses a non-admin application credential with the service role.
// Only its own service token is cached. User tokens are validated on every call.
type Keystone struct {
	url        string
	client     *http.Client
	credential config.ServiceCredential
	mu         sync.Mutex
	token      string
	expires    time.Time
	refresh    *tokenRefresh
}

type tokenRefresh struct {
	done  chan struct{}
	token string
	err   error
}

func NewKeystone(url string, client *http.Client, credential config.ServiceCredential) *Keystone {
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Keystone{url: strings.TrimRight(url, "/") + "/auth/tokens", client: &copyClient, credential: credential}
}

type tokenResponse struct {
	Token struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
		Project struct {
			ID string `json:"id"`
		} `json:"project"`
		Roles []struct {
			Name string `json:"name"`
		} `json:"roles"`
		ExpiresAt time.Time `json:"expires_at"`
	} `json:"token"`
}

func readIdentity(body io.Reader) (Identity, error) {
	data, err := io.ReadAll(io.LimitReader(body, 1048577))
	if err != nil || len(data) > 1048576 {
		return Identity{}, ErrUnavailable
	}
	var decoded tokenResponse
	if json.Unmarshal(data, &decoded) != nil {
		return Identity{}, ErrUnavailable
	}
	identity := Identity{UserID: decoded.Token.User.ID, ProjectID: decoded.Token.Project.ID, ExpiresAt: decoded.Token.ExpiresAt}
	for _, role := range decoded.Token.Roles {
		if role.Name == "" {
			return Identity{}, ErrUnavailable
		}
		identity.Roles = append(identity.Roles, role.Name)
	}
	if err := identity.Check(time.Now()); err != nil {
		return Identity{}, err
	}
	return identity, nil
}

func validHeaderToken(token string) bool {
	if token == "" || len(token) > 16384 {
		return false
	}
	for _, c := range token {
		if c <= 32 || c >= 127 {
			return false
		}
	}
	return true
}

func (k *Keystone) serviceToken(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", errors.Join(ErrUnavailable, err)
	}
	k.mu.Lock()
	if k.token != "" && time.Now().Add(30*time.Second).Before(k.expires) {
		token := k.token
		k.mu.Unlock()
		return token, nil
	}
	if flight := k.refresh; flight != nil {
		k.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", errors.Join(ErrUnavailable, ctx.Err())
		case <-flight.done:
			if err := ctx.Err(); err != nil {
				return "", errors.Join(ErrUnavailable, err)
			}
			return flight.token, flight.err
		}
	}
	flight := &tokenRefresh{done: make(chan struct{})}
	k.refresh = flight
	k.mu.Unlock()
	token, expires, err := k.fetchServiceToken(ctx)
	if ctx.Err() != nil {
		err = errors.Join(ErrUnavailable, ctx.Err())
	}
	k.mu.Lock()
	if err == nil {
		k.token, k.expires = token, expires
	}
	flight.token, flight.err = token, err
	k.refresh = nil
	close(flight.done)
	k.mu.Unlock()
	return token, err
}

func (k *Keystone) fetchServiceToken(ctx context.Context) (string, time.Time, error) {
	if k.credential.ID == "" || k.credential.Secret == "" {
		return "", time.Time{}, ErrUnavailable
	}
	body := map[string]any{"auth": map[string]any{"identity": map[string]any{"methods": []string{"application_credential"}, "application_credential": map[string]string{"id": k.credential.ID, "secret": k.credential.Secret}}}}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", time.Time{}, ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, k.url, bytes.NewReader(encoded))
	if err != nil {
		return "", time.Time{}, ErrUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	response, err := k.client.Do(req)
	if err != nil {
		return "", time.Time{}, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return "", time.Time{}, ErrUnavailable
	}
	identity, err := readIdentity(response.Body)
	if err != nil || !identity.HasRole("service") || identity.HasRole("admin") {
		return "", time.Time{}, ErrUnavailable
	}
	token := response.Header.Get("X-Subject-Token")
	if !validHeaderToken(token) {
		return "", time.Time{}, ErrUnavailable
	}
	return token, identity.ExpiresAt, nil
}

func (k *Keystone) invalidate(token string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.token == token {
		k.token = ""
		k.expires = time.Time{}
	}
}

func (k *Keystone) Authenticate(ctx context.Context, token string) (Identity, error) {
	if !validHeaderToken(token) {
		return Identity{}, ErrInvalidToken
	}
	for attempt := 0; attempt < 2; attempt++ {
		serviceToken, err := k.serviceToken(ctx)
		if err != nil {
			return Identity{}, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.url+"?nocatalog", nil)
		if err != nil {
			return Identity{}, ErrUnavailable
		}
		req.Header.Set("X-Auth-Token", serviceToken)
		req.Header.Set("X-Subject-Token", token)
		req.Header.Set("Accept", "application/json")
		response, err := k.client.Do(req)
		if err != nil {
			return Identity{}, ErrUnavailable
		}
		if response.StatusCode == http.StatusUnauthorized {
			response.Body.Close()
			k.invalidate(serviceToken)
			continue
		}
		if response.StatusCode == http.StatusNotFound {
			response.Body.Close()
			return Identity{}, ErrInvalidToken
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return Identity{}, ErrUnavailable
		}
		identity, err := readIdentity(response.Body)
		response.Body.Close()
		return identity, err
	}
	return Identity{}, ErrUnavailable
}
