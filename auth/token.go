package auth

import (
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func GenerateTokens(userID string) (accessToken string, refreshToken string, err error) {
	jwtSecret := []byte(os.Getenv("JWT_SECRET"))

	// Access Token (15 min - 1 hr)
	access := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(time.Minute * 15).Unix(), // SHORT lifespan
		"typ": "access",
	})

	accessToken, err = access.SignedString(jwtSecret)
	if err != nil {
		return "", "", fmt.Errorf("failed to sign access token: %v", err)
	}

	// Refresh Token (7 - 30 days)
	refresh := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(time.Hour * 24 * 30).Unix(), // LONG lifespan
		"typ": "refresh",
	})

	refreshToken, err = refresh.SignedString(jwtSecret)
	if err != nil {
		return "", "", fmt.Errorf("failed to sign refresh token: %v", err)
	}

	return accessToken, refreshToken, nil
}

func ValidateToken(tokenStr string) (string, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to parse token: %v", err)
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		userID, ok := claims["sub"].(string)
		if !ok {
			return "", fmt.Errorf("invalid sub claim")
		}
		return userID, nil
	}

	return "", fmt.Errorf("invalid token")
}
