package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/basit/fileshare-backend/auth"
	"github.com/basit/fileshare-backend/graph/resolvers"
	//	"github.com/basit/fileshare-backend/graph/resolvers"
)

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")

		// ✅ If no auth header, just continue (unauthenticated access)
		if authHeader == "" {
			next.ServeHTTP(w, r)
			return
		}

		// ✅ Handle Bearer token format gracefully
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 && parts[0] == "Bearer" {
			token := parts[1]

			userID, err := auth.ValidateToken(token)
			if err == nil {
				if parsedUID, err := uuid.Parse(userID); err == nil {
					ctx := context.WithValue(r.Context(), resolvers.UserIDKey, parsedUID)
					r = r.WithContext(ctx)
				}
			}
			// ❌ Else: Invalid token or userID — ignore, continue unauthenticated
		}

		// ✅ Continue no matter what
		next.ServeHTTP(w, r)
	})
}

const GinContextKey = "GinContextKey"

func GinContextToContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), GinContextKey, c)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func GetGinContext(ctx context.Context) (*gin.Context, error) {
	ginContext := ctx.Value(GinContextKey)
	if ginContext == nil {
		return nil, fmt.Errorf("could not get gin context")
	}
	gc, ok := ginContext.(*gin.Context)
	if !ok {
		return nil, fmt.Errorf("gin context has wrong type")
	}
	return gc, nil
}
