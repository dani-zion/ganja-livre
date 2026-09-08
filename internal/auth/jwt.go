package auth

import (
	"context"
	"errors"
	"time"

	"github.com/dani-zion/ganja-livre/internal/config"
	"github.com/dani-zion/ganja-livre/internal/graph/model"
	"github.com/golang-jwt/jwt/v5"
)

// contextKey is an unexported type to avoid key collisions in context.
type contextKey string

const UserClaimsKey contextKey = "user_claims"

// TokenPair holds the access and refresh tokens.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

// Claims holds the JWT payload fields exposed to the application.
type Claims struct {
	UserID string         `json:"uid"`
	Email  string         `json:"email"`
	Role   model.UserRole `json:"role"`
	jwt.RegisteredClaims
}

// Service handles JWT creation and validation.
type Service struct {
	cfg config.JWTConfig
}

func NewService(cfg config.JWTConfig) *Service {
	return &Service{cfg: cfg}
}

// IssueTokenPair creates a fresh access + refresh token pair.
func (s *Service) IssueTokenPair(userID, email string, role model.UserRole) (*TokenPair, error) {
	access, err := s.sign(userID, email, role, s.cfg.AccessTokenExpiry, s.cfg.AccessSecret)
	if err != nil {
		return nil, err
	}
	refresh, err := s.sign(userID, email, role, s.cfg.RefreshTokenExpiry, s.cfg.RefreshSecret)
	if err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: access, RefreshToken: refresh}, nil
}

// ValidateAccessToken parses and validates an access token.
func (s *Service) ValidateAccessToken(tokenStr string) (*Claims, error) {
	return s.validate(tokenStr, s.cfg.AccessSecret)
}

// ValidateRefreshToken parses and validates a refresh token.
func (s *Service) ValidateRefreshToken(tokenStr string) (*Claims, error) {
	return s.validate(tokenStr, s.cfg.RefreshSecret)
}

// ClaimsFromContext extracts user claims from a context.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(UserClaimsKey).(*Claims)
	return c, ok
}

// ContextWithClaims injects claims into a context (used by middleware).
func ContextWithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, UserClaimsKey, claims)
}

// ─── Private ─────────────────────────────────────────────────────────────────

func (s *Service) sign(userID, email string, role model.UserRole, expiry time.Duration, secret string) (string, error) {
	now := time.Now().UTC()
	claims := &Claims{
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
			Issuer:    "ganja-livre",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func (s *Service) validate(tokenStr, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{"HS256"}))

	if err != nil {
		return nil, err
	}

	cc, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	return &Claims{
		UserID: cc.UserID,
		Email:  cc.Email,
		Role:   cc.Role,
	}, nil
}
