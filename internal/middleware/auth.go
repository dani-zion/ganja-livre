package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/dani-zion/ganja-livre/internal/auth"
	"github.com/dani-zion/ganja-livre/internal/graph/model"
	"go.uber.org/zap"
)

// Auth extracts the Bearer token from the Authorization header,
// validates it, and injects the claims into the request context.
// Requests without a token are allowed through — resolvers enforce auth.
func Auth(jwtSvc *auth.Service, log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				next.ServeHTTP(w, r)
				return
			}

			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				http.Error(w, "malformed authorization header", http.StatusUnauthorized)
				return
			}

			claims, err := jwtSvc.ValidateAccessToken(parts[1])
			if err != nil {
				log.Debug("invalid token", zap.Error(err))
				http.Error(w, "invalid or expired token", http.StatusUnauthorized)
				return
			}

			ctx := auth.ContextWithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Security headers applied to every response.
func SecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; font-src https://cdn.jsdelivr.net; img-src 'self' data:;")
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			next.ServeHTTP(w, r)
		})
	}
}

// ─── RBAC helpers used inside resolvers ──────────────────────────────────────

// RequireAuth returns the claims from context or an UNAUTHENTICATED error.
func RequireAuth(ctx context.Context) (*auth.Claims, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok || claims == nil {
		return nil, ErrUnauthenticated
	}
	return claims, nil
}

// RequireRole checks that the authenticated user has one of the allowed roles.
func RequireRole(ctx context.Context, roles ...model.UserRole) (*auth.Claims, error) {
	claims, err := RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range roles {
		if claims.Role == r {
			return claims, nil
		}
	}
	return nil, ErrForbidden
}

// ─── Sentinel errors ─────────────────────────────────────────────────────────

var (
	ErrUnauthenticated = newGQLError("UNAUTHENTICATED", "authentication required")
	ErrForbidden       = newGQLError("FORBIDDEN", "insufficient permissions")
)

type gqlError struct {
	code    string
	message string
}

func (e *gqlError) Error() string { return e.message }
func (e *gqlError) Extensions() map[string]interface{} {
	return map[string]interface{}{"code": e.code}
}

func newGQLError(code, msg string) *gqlError {
	return &gqlError{code: code, message: msg}
}
