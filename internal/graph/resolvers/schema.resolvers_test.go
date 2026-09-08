package resolvers

import (
	"context"
	"testing"
	"time"

	"github.com/dani-zion/ganja-livre/internal/auth"
	"github.com/dani-zion/ganja-livre/internal/config"
	"github.com/dani-zion/ganja-livre/internal/graph/model"
	"github.com/dani-zion/ganja-livre/internal/mongodb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
	"golang.org/x/crypto/bcrypt"
)

var testJWTConfig = config.JWTConfig{
	AccessSecret:       "test-access-secret",
	RefreshSecret:      "test-refresh-secret",
	AccessTokenExpiry:  15 * time.Minute,
	RefreshTokenExpiry: 7 * 24 * time.Hour,
}

func registerResolver(mt *mtest.T) *Resolver {
	mt.Helper()
	return &Resolver{
		cols:   &mongodb.Collections{Users: mt.Coll},
		jwtSvc: auth.NewService(testJWTConfig),
	}
}

func doRegister(r *Resolver, input model.RegisterInput) (*model.AuthPayload, error) {
	return (&mutationResolver{r}).Register(context.Background(), input)
}

func TestRegister_Success(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("success", func(mt *mtest.T) {
		r := registerResolver(mt)
		mt.AddMockResponses(mtest.CreateSuccessResponse())

		payload, err := doRegister(r, model.RegisterInput{
			Email:    "user@example.com",
			Password: "securepass123",
			Name:     "Test User",
		})

		require.NoError(t, err)
		assert.NotEmpty(t, payload.AccessToken)
		assert.NotEmpty(t, payload.RefreshToken)
		assert.Equal(t, "user@example.com", payload.User.Email)
		assert.Equal(t, "Test User", payload.User.Name)
		assert.Equal(t, model.UserRoleCustomer, payload.User.Role)
		assert.False(t, payload.User.CreatedAt.IsZero())
	})
}

func TestRegister_EmptyEmail(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("empty_email", func(mt *mtest.T) {
		r := registerResolver(mt)
		_, err := doRegister(r, model.RegisterInput{
			Email:    "",
			Password: "securepass123",
			Name:     "Test User",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "email, password, and name are required")
	})
}

func TestRegister_EmptyPassword(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("empty_password", func(mt *mtest.T) {
		r := registerResolver(mt)
		_, err := doRegister(r, model.RegisterInput{
			Email:    "user@example.com",
			Password: "",
			Name:     "Test User",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "email, password, and name are required")
	})
}

func TestRegister_EmptyName(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("empty_name", func(mt *mtest.T) {
		r := registerResolver(mt)
		_, err := doRegister(r, model.RegisterInput{
			Email:    "user@example.com",
			Password: "securepass123",
			Name:     "",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "email, password, and name are required")
	})
}

func TestRegister_WhitespaceOnlyEmail(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("whitespace_only_email", func(mt *mtest.T) {
		r := registerResolver(mt)
		_, err := doRegister(r, model.RegisterInput{
			Email:    "   ",
			Password: "securepass123",
			Name:     "Test User",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "email, password, and name are required")
	})
}

func TestRegister_ShortPassword(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("short_password", func(mt *mtest.T) {
		r := registerResolver(mt)
		_, err := doRegister(r, model.RegisterInput{
			Email:    "user@example.com",
			Password: "abc",
			Name:     "Test User",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "password must be at least 8 characters")
	})
}

func TestRegister_DuplicateEmail(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("duplicate_email", func(mt *mtest.T) {
		r := registerResolver(mt)
		mt.AddMockResponses(mtest.CreateSuccessResponse())

		_, err := doRegister(r, model.RegisterInput{
			Email:    "dup@example.com",
			Password: "securepass123",
			Name:     "First User",
		})
		require.NoError(t, err)

		mt.AddMockResponses(bson.D{
			{Key: "ok", Value: 0},
			{Key: "code", Value: 11000},
			{Key: "errmsg", Value: "duplicate key error"},
		})

		_, err = doRegister(r, model.RegisterInput{
			Email:    "dup@example.com",
			Password: "securepass456",
			Name:     "Second User",
		})

		require.Error(t, err)
		assert.Equal(t, errEmailTaken, err)
	})
}

func TestRegister_PasswordIsHashed(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("password_is_hashed", func(mt *mtest.T) {
		r := registerResolver(mt)
		mt.AddMockResponses(mtest.CreateSuccessResponse())

		_, err := doRegister(r, model.RegisterInput{
			Email:    "hash@example.com",
			Password: "mypassword123",
			Name:     "Hash Test",
		})
		require.NoError(t, err)

		hash, err := bcrypt.GenerateFromPassword([]byte("mypassword123"), bcrypt.DefaultCost)
		require.NoError(t, err)
		assert.NoError(t, bcrypt.CompareHashAndPassword(hash, []byte("mypassword123")))
		assert.Error(t, bcrypt.CompareHashAndPassword(hash, []byte("wrongpassword")))
	})
}

func TestRegister_TokensAreValid(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("tokens_are_valid", func(mt *mtest.T) {
		r := registerResolver(mt)
		mt.AddMockResponses(mtest.CreateSuccessResponse())

		payload, err := doRegister(r, model.RegisterInput{
			Email:    "token@example.com",
			Password: "securepass123",
			Name:     "Token Test",
		})
		require.NoError(t, err)

		accessClaims, err := r.jwtSvc.ValidateAccessToken(payload.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, payload.User.Email, accessClaims.Email)
		assert.Equal(t, model.UserRoleCustomer, accessClaims.Role)

		refreshClaims, err := r.jwtSvc.ValidateRefreshToken(payload.RefreshToken)
		require.NoError(t, err)
		assert.Equal(t, payload.User.Email, refreshClaims.Email)
	})
}

// ─── Login tests ─────────────────────────────────────────────────────────────

func doLogin(r *Resolver, input model.LoginInput) (*model.AuthPayload, error) {
	return (&mutationResolver{r}).Login(context.Background(), input)
}

func userDoc(email, passwordHash, name, role string) bson.D {
	return bson.D{
		{Key: "_id", Value: primitive.NewObjectID()},
		{Key: "email", Value: email},
		{Key: "password_hash", Value: passwordHash},
		{Key: "name", Value: name},
		{Key: "role", Value: role},
		{Key: "is_active", Value: true},
		{Key: "created_at", Value: time.Now().UTC()},
		{Key: "updated_at", Value: time.Now().UTC()},
	}
}

func TestLogin_Success(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("success", func(mt *mtest.T) {
		r := registerResolver(mt)
		hash, _ := bcrypt.GenerateFromPassword([]byte("mypassword"), bcrypt.DefaultCost)
		doc := userDoc("login@example.com", string(hash), "Login User", "CUSTOMER")

		mt.AddMockResponses(mtest.CreateCursorResponse(1, "ganja-livre.users", mtest.FirstBatch, doc))

		payload, err := doLogin(r, model.LoginInput{
			Email:    "login@example.com",
			Password: "mypassword",
		})

		require.NoError(t, err)
		assert.NotEmpty(t, payload.AccessToken)
		assert.NotEmpty(t, payload.RefreshToken)
		assert.Equal(t, "login@example.com", payload.User.Email)
		assert.Equal(t, "Login User", payload.User.Name)
		assert.Equal(t, model.UserRoleCustomer, payload.User.Role)
	})
}

func TestLogin_EmptyEmail(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("empty_email", func(mt *mtest.T) {
		r := registerResolver(mt)
		_, err := doLogin(r, model.LoginInput{
			Email:    "",
			Password: "mypassword",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "email and password are required")
	})
}

func TestLogin_EmptyPassword(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("empty_password", func(mt *mtest.T) {
		r := registerResolver(mt)
		_, err := doLogin(r, model.LoginInput{
			Email:    "login@example.com",
			Password: "",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "email and password are required")
	})
}

func TestLogin_UserNotFound(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("user_not_found", func(mt *mtest.T) {
		r := registerResolver(mt)
		mt.AddMockResponses(mtest.CreateCursorResponse(0, "ganja-livre.users", mtest.FirstBatch))

		_, err := doLogin(r, model.LoginInput{
			Email:    "nobody@example.com",
			Password: "mypassword",
		})

		require.Error(t, err)
		assert.Equal(t, errInvalidCredentials, err)
	})
}

func TestLogin_WrongPassword(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("wrong_password", func(mt *mtest.T) {
		r := registerResolver(mt)
		hash, _ := bcrypt.GenerateFromPassword([]byte("correctpassword"), bcrypt.DefaultCost)
		doc := userDoc("login@example.com", string(hash), "Login User", "CUSTOMER")

		mt.AddMockResponses(mtest.CreateCursorResponse(1, "ganja-livre.users", mtest.FirstBatch, doc))

		_, err := doLogin(r, model.LoginInput{
			Email:    "login@example.com",
			Password: "wrongpassword",
		})

		require.Error(t, err)
		assert.Equal(t, errInvalidCredentials, err)
	})
}

func TestLogin_WhitespaceEmailTrimmed(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("whitespace_email_trimmed", func(mt *mtest.T) {
		r := registerResolver(mt)
		hash, _ := bcrypt.GenerateFromPassword([]byte("mypassword"), bcrypt.DefaultCost)
		doc := userDoc("login@example.com", string(hash), "Login User", "CUSTOMER")

		mt.AddMockResponses(mtest.CreateCursorResponse(0, "ganja-livre.users", mtest.FirstBatch, doc))

		payload, err := doLogin(r, model.LoginInput{
			Email:    "  login@example.com  ",
			Password: "mypassword",
		})

		require.NoError(t, err)
		assert.Equal(t, "login@example.com", payload.User.Email)
	})
}

func TestLogin_TokensAreValid(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("tokens_are_valid", func(mt *mtest.T) {
		r := registerResolver(mt)
		hash, _ := bcrypt.GenerateFromPassword([]byte("mypassword"), bcrypt.DefaultCost)
		doc := userDoc("token@example.com", string(hash), "Token User", "SELLER")

		mt.AddMockResponses(mtest.CreateCursorResponse(0, "ganja-livre.users", mtest.FirstBatch, doc))

		payload, err := doLogin(r, model.LoginInput{
			Email:    "token@example.com",
			Password: "mypassword",
		})
		require.NoError(t, err)

		accessClaims, err := r.jwtSvc.ValidateAccessToken(payload.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, payload.User.Email, accessClaims.Email)
		assert.Equal(t, model.UserRoleSeller, accessClaims.Role)

		refreshClaims, err := r.jwtSvc.ValidateRefreshToken(payload.RefreshToken)
		require.NoError(t, err)
		assert.Equal(t, payload.User.Email, refreshClaims.Email)
	})
}
