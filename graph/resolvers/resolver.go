package resolvers

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Resolver holds dependencies and implements ResolverRoot
type Resolver struct {
	// Add any dependencies your resolvers need, for example:
	// db *gorm.DB
}

// Key type for context values
type contextKey string

const UserIDKey contextKey = "userID"

// Helper functions for auth context
func GetUserIDFromContext(ctx context.Context) (*uuid.UUID, error) {
	value := ctx.Value(UserIDKey)
	if value == nil {
		return nil, fmt.Errorf("unauthorized")
	}

	userID, ok := value.(uuid.UUID)
	if !ok {
		return nil, fmt.Errorf("invalid user ID format")
	}

	return &userID, nil
}
