package resolvers

import (
	"context"
	"time"

	"github.com/dani-zion/ganja-livre/internal/graph/model"
	"github.com/dani-zion/ganja-livre/internal/middleware"
	dbmodel "github.com/dani-zion/ganja-livre/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// BecomeSeller adds the SELLER role to the authenticated user.
func (r *mutationResolver) BecomeSeller(ctx context.Context) (*model.User, error) {
	claims, err := middleware.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	uid, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return nil, errNotFound
	}

	var user dbmodel.User
	err = r.cols.Users.FindOne(ctx, bson.M{"_id": uid}).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return nil, errNotFound
	}
	if err != nil {
		return nil, errInternal
	}

	for _, role := range user.Roles {
		if role == dbmodel.RoleSeller {
			roles := toGraphRoles(user.Roles)
			return &model.User{
				ID:        user.ID.Hex(),
				Email:     user.Email,
				Name:      user.Name,
				Roles:     roles,
				CreatedAt: user.CreatedAt,
				UpdatedAt: user.UpdatedAt,
			}, nil
		}
	}

	newRoles := append(user.Roles, dbmodel.RoleSeller)
	now := time.Now().UTC()
	_, err = r.cols.Users.UpdateOne(ctx,
		bson.M{"_id": uid},
		bson.M{"$set": bson.M{"roles": newRoles, "updated_at": now}},
	)
	if err != nil {
		return nil, errInternal
	}

	user.Roles = newRoles
	user.UpdatedAt = now
	roles := toGraphRoles(user.Roles)

	return &model.User{
		ID:        user.ID.Hex(),
		Email:     user.Email,
		Name:      user.Name,
		Roles:     roles,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}, nil
}

// StopSelling removes the SELLER role from the authenticated user.
func (r *mutationResolver) StopSelling(ctx context.Context) (*model.User, error) {
	claims, err := middleware.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	uid, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return nil, errNotFound
	}

	var user dbmodel.User
	err = r.cols.Users.FindOne(ctx, bson.M{"_id": uid}).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return nil, errNotFound
	}
	if err != nil {
		return nil, errInternal
	}

	var newRoles []dbmodel.UserRole
	for _, role := range user.Roles {
		if role != dbmodel.RoleSeller {
			newRoles = append(newRoles, role)
		}
	}

	if len(newRoles) == 0 {
		newRoles = []dbmodel.UserRole{dbmodel.RoleCustomer}
	}

	now := time.Now().UTC()
	_, err = r.cols.Users.UpdateOne(ctx,
		bson.M{"_id": uid},
		bson.M{"$set": bson.M{"roles": newRoles, "updated_at": now}},
	)
	if err != nil {
		return nil, errInternal
	}

	user.Roles = newRoles
	user.UpdatedAt = now
	roles := toGraphRoles(user.Roles)

	return &model.User{
		ID:        user.ID.Hex(),
		Email:     user.Email,
		Name:      user.Name,
		Roles:     roles,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}, nil
}
