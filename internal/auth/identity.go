// Package auth validates opaque Keystone tokens; client identity headers are not trusted.
package auth

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidToken = errors.New("authentication token is missing, invalid or expired")
	ErrUnavailable  = errors.New("identity validation is unavailable")
	ErrForbidden    = errors.New("a project-scoped identity is required")
)

type Identity struct {
	UserID, ProjectID string
	Roles             []string
	ExpiresAt         time.Time
	// Only Keystone's composite validation can grant expired-user access.
	// ExpiresAt remains the user's original expiry; service roles are not copied.
	serviceExpiresAt time.Time
}

func (i Identity) HasRole(role string) bool {
	for _, candidate := range i.Roles {
		if candidate == role {
			return true
		}
	}
	return false
}

func (i Identity) Check(now time.Time) error {
	if i.UserID == "" || i.ExpiresAt.IsZero() {
		return ErrUnavailable
	}
	if !i.serviceExpiresAt.IsZero() && !now.Before(i.serviceExpiresAt) {
		return ErrInvalidToken
	}
	if !now.Before(i.ExpiresAt) && i.serviceExpiresAt.IsZero() {
		return ErrInvalidToken
	}
	if i.ProjectID == "" {
		return ErrForbidden
	}
	return nil
}

type Authenticator interface {
	Authenticate(context.Context, string) (Identity, error)
}

// CompositeAuthenticator validates the two credentials independently and returns
// only the original user's identity. Implementations must not merge service roles.
type CompositeAuthenticator interface {
	Authenticator
	AuthenticateComposite(context.Context, string, string) (Identity, error)
}
