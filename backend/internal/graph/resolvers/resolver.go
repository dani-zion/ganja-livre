package resolvers

import (
	"errors"

	"github.com/dani-zion/ganja-livre/internal/auth"
	"github.com/dani-zion/ganja-livre/internal/mongodb"
	"go.temporal.io/sdk/client"
)

// Resolver is the root resolver injected with all dependencies.
// All sub-resolvers are methods on this type.
type Resolver struct {
	cols     *mongodb.Collections
	jwtSvc   *auth.Service
	temporal client.Client
}

// New constructs the root resolver with required dependencies.
func New(cols *mongodb.Collections, jwtSvc *auth.Service, temporalClient client.Client) *Resolver {
	return &Resolver{
		cols:     cols,
		jwtSvc:   jwtSvc,
		temporal: temporalClient,
	}
}

// ─── Sentinel errors (GQL-friendly) ─────────────────────────────────────────

var (
	errInternal           = errors.New("internal server error")
	errNotFound           = errors.New("resource not found")
	errEmailTaken         = errors.New("email already registered")
	errInvalidToken       = errors.New("invalid or expired token")
	errInvalidCredentials = errors.New("invalid email or password")
)
