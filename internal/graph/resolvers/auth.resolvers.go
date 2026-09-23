package resolvers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dani-zion/ganja-livre/internal/graph/model"
	dbmodel "github.com/dani-zion/ganja-livre/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
)

// Register new users.
func (r *mutationResolver) Register(ctx context.Context, input model.RegisterInput) (*model.AuthPayload, error) {
	input.Email = strings.TrimSpace(input.Email)
	input.Name = strings.TrimSpace(input.Name)

	if input.Email == "" || input.Password == "" || input.Name == "" {
		return nil, fmt.Errorf("email, password, and name are required")
	}
	if len(input.Password) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errInternal
	}

	roles := input.Roles
	if len(roles) == 0 {
		roles = []model.UserRole{model.UserRoleCustomer}
	}

	now := time.Now().UTC()
	user := dbmodel.User{
		ID:           primitive.NewObjectID(),
		Email:        input.Email,
		PasswordHash: string(hash),
		Name:         input.Name,
		Roles:        toDBRoles(roles),
		IsActive:     true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	_, err = r.cols.Users.InsertOne(ctx, user)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, errEmailTaken
		}
		return nil, errInternal
	}

	tokens, err := r.jwtSvc.IssueTokenPair(user.ID.Hex(), user.Email, roles)
	if err != nil {
		return nil, errInternal
	}

	return &model.AuthPayload{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		User: &model.User{
			ID:        user.ID.Hex(),
			Email:     user.Email,
			Name:      user.Name,
			Roles:     roles,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
		},
	}, nil
}

// Login is the resolver for the login field.
func (r *mutationResolver) Login(ctx context.Context, input model.LoginInput) (*model.AuthPayload, error) {
	input.Email = strings.TrimSpace(input.Email)

	if input.Email == "" || input.Password == "" {
		return nil, fmt.Errorf("email and password are required")
	}

	var user dbmodel.User
	err := r.cols.Users.FindOne(ctx, bson.M{"email": input.Email}).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return nil, errInvalidCredentials
	}
	if err != nil {
		return nil, errInternal
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)); err != nil {
		return nil, errInvalidCredentials
	}

	roles := toGraphRoles(user.Roles)
	tokens, err := r.jwtSvc.IssueTokenPair(user.ID.Hex(), user.Email, roles)
	if err != nil {
		return nil, errInternal
	}

	return &model.AuthPayload{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		User: &model.User{
			ID:        user.ID.Hex(),
			Email:     user.Email,
			Name:      user.Name,
			Roles:     roles,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
		},
	}, nil
}

// RefreshToken is the resolver for the refreshToken field.
func (r *mutationResolver) RefreshToken(ctx context.Context, token string) (*model.AuthPayload, error) {
	claims, err := r.jwtSvc.ValidateRefreshToken(token)
	if err != nil {
		return nil, errInvalidToken
	}

	uid, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return nil, errInvalidToken
	}

	var user dbmodel.User
	err = r.cols.Users.FindOne(ctx, bson.M{"_id": uid}).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return nil, errNotFound
	}
	if err != nil {
		return nil, errInternal
	}

	if !user.IsActive {
		return nil, errInvalidToken
	}

	roles := toGraphRoles(user.Roles)
	tokens, err := r.jwtSvc.IssueTokenPair(user.ID.Hex(), user.Email, roles)
	if err != nil {
		return nil, errInternal
	}

	return &model.AuthPayload{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		User: &model.User{
			ID:        user.ID.Hex(),
			Email:     user.Email,
			Name:      user.Name,
			Roles:     roles,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
		},
	}, nil
}
