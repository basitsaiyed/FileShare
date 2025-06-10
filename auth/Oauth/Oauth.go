package Oauth

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/basit/fileshare-backend/auth"
	"github.com/basit/fileshare-backend/initializers"
	"github.com/basit/fileshare-backend/models"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/google"
	"gorm.io/gorm"
)

type TokenResponse struct {
	OauthAccessToken  string `json:"oauth_access_token"`
	ExpiresIn         int    `json:"expires_in"`
	TokenType         string `json:"token_type"`
	OauthRefreshToken string `json:"oauth_refresh_token,omitempty"` // Sometimes returned
}

func InitStore() {
	sessionSecret := os.Getenv("SESSION_SECRET")
	if sessionSecret == "" {
		log.Fatal("SESSION_SECRET is not set")
	}

	fmt.Println("Session Key:", sessionSecret)

	store := cookie.NewStore([]byte(sessionSecret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,
		Secure:   true, // Set to true in production with HTTPS
	})

	gothic.Store = store

	fmt.Println("GOOGLE_CLIENT_ID", os.Getenv("GOOGLE_CLIENT_ID"))
	fmt.Println("GOOGLE_CLIENT_SECRET", os.Getenv("GOOGLE_CLIENT_SECRET"))
	fmt.Println("GOOGLE_REDIRECT_URL", os.Getenv("GOOGLE_REDIRECT_URL"))
	provider := google.New(
		os.Getenv("GOOGLE_CLIENT_ID"),
		os.Getenv("GOOGLE_CLIENT_SECRET"),
		os.Getenv("GOOGLE_REDIRECT_URL"),
		"https://www.googleapis.com/auth/calendar.events",
		"https://www.googleapis.com/auth/gmail.readonly",
		"https://www.googleapis.com/auth/gmail.send",
		"email",
		"profile",
	)

	// Force refresh token by adding extra auth parameters
	provider.SetAccessType("offline") // Ensures refresh token is received
	provider.SetPrompt("consent")

	goth.UseProviders(provider)
}

// Begin OAuth authentication
func OauthCallbackHandler(c *gin.Context) {
	log.Println("🌐 OAuth callback reached: ", c.Request.URL.String())

	provider := c.Param("provider")

	// For Google, add access_type=offline to get refresh token
	if provider == "google" {
		q := c.Request.URL.Query()
		q.Add("access_type", "offline")
		q.Add("prompt", "consent") // Force consent screen to get refresh token
		c.Request.URL.RawQuery = q.Encode()
	}

	// For GitHub, add scope to get user email (if public)
	if provider == "github" {
		q := c.Request.URL.Query()
		q.Add("scope", "user:email") // Request email access
		c.Request.URL.RawQuery = q.Encode()
	}

	// Goth expects the provider to be in the URL path
	q := c.Request.URL.Query()
	q.Add("provider", provider)
	c.Request.URL.RawQuery = q.Encode()

	gothic.BeginAuthHandler(c.Writer, c.Request)
}

// Complete OAuth authentication
func CompleteAuth(c *gin.Context) {
	// Add provider to query params for goth
	q := c.Request.URL.Query()
	q.Add("provider", c.Param("provider"))
	c.Request.URL.RawQuery = q.Encode()

	gothUser, err := gothic.CompleteUserAuth(c.Writer, c.Request)
	if err != nil {
		log.Printf("Auth error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Find or create user in database
	user, err := findOrCreateOAuthUser(gothUser)
	if err != nil {
		log.Printf("Database error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process user data"})
		return
	}

	// Log OAuth user data
	log.Printf("Provider: %s", gothUser.Provider)
	log.Printf("User ID: %s", gothUser.UserID)
	log.Printf("User Email: %s", gothUser.Email)
	log.Printf("User Name: %s", gothUser.Name)

	// Generate JWT tokens using the user's UUID
	accessToken, refreshToken, err := auth.GenerateTokens(user.ID.String())
	if err != nil {
		log.Printf("Token generation error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
		return
	}

	// Set refresh token as secure HTTP-only cookie
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		HttpOnly: true,
		Secure:   true, // Set to true in production with HTTPS
		Path:     "/api/refresh-token",
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(30 * 24 * time.Hour),
	})

	// Store minimal session data (optional - you might not need this if using JWT)
	session := sessions.Default(c)
	session.Set("authenticated", true)
	session.Set("user_id", user.ID.String())
	if err := session.Save(); err != nil {
		log.Printf("Session save error: %v", err)
		// Don't fail the auth process for session errors
	}

	log.Printf("OAuth authentication successful for user: %s", user.Email)

	// Redirect to your frontend with the access token
	// You can either redirect to a success page or return JSON
	frontendURL := os.Getenv("BASE_URL") // Replace with your frontend URL
	redirectURL := fmt.Sprintf("%s/auth/success?token=%s", frontendURL, accessToken)
	c.Redirect(http.StatusTemporaryRedirect, redirectURL)
}

func findOrCreateOAuthUser(gothUser goth.User) (*models.User, error) {
	var user models.User

	// Try to find existing user by OAuth provider ID first
	var err error
	switch gothUser.Provider {
	case "google":
		err = initializers.DB.Where("google_id = ?", gothUser.UserID).First(&user).Error
	case "github":
		err = initializers.DB.Where("git_hub_id = ?", gothUser.UserID).First(&user).Error
	default:
		return nil, fmt.Errorf("unsupported provider: %s", gothUser.Provider)
	}

	if err == nil {
		// User exists, update OAuth tokens
		return updateExistingOAuthUser(&user, gothUser)
	}

	if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("database query error: %v", err)
	}

	// User doesn't exist by OAuth ID, try to find by email
	err = initializers.DB.Where("email = ?", gothUser.Email).First(&user).Error
	if err == nil {
		// User exists with this email, link OAuth account
		return linkOAuthToExistingUser(&user, gothUser)
	}

	if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("database query error: %v", err)
	}

	// Create new user
	return createNewOAuthUser(gothUser)
}

func updateExistingOAuthUser(user *models.User, gothUser goth.User) (*models.User, error) {
	// Update user's OAuth tokens and info
	updates := map[string]interface{}{
		"name":       gothUser.Name,
		"avatar_url": gothUser.AvatarURL,
	}

	switch gothUser.Provider {
	case "google":
		updates["google_access_token"] = gothUser.AccessToken
		if gothUser.RefreshToken != "" {
			updates["google_refresh_token"] = gothUser.RefreshToken
		}
		if !gothUser.ExpiresAt.IsZero() {
			updates["google_token_expires_at"] = gothUser.ExpiresAt
		}
	case "github":
		updates["git_hub_access_token"] = gothUser.AccessToken
		if !gothUser.ExpiresAt.IsZero() {
			updates["git_hub_token_expires_at"] = gothUser.ExpiresAt
		}
	}

	if err := initializers.DB.Model(user).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update user: %v", err)
	}

	return user, nil
}

func linkOAuthToExistingUser(user *models.User, gothUser goth.User) (*models.User, error) {
	// Link OAuth account to existing user
	updates := map[string]interface{}{
		"name":       gothUser.Name,
		"avatar_url": gothUser.AvatarURL,
		"provider":   gothUser.Provider,
	}

	switch gothUser.Provider {
	case "google":
		updates["google_id"] = gothUser.UserID
		updates["google_access_token"] = gothUser.AccessToken
		if gothUser.RefreshToken != "" {
			updates["google_refresh_token"] = gothUser.RefreshToken
		}
		if !gothUser.ExpiresAt.IsZero() {
			updates["google_token_expires_at"] = gothUser.ExpiresAt
		}
	case "github":
		updates["git_hub_id"] = gothUser.UserID
		updates["git_hub_access_token"] = gothUser.AccessToken
		if !gothUser.ExpiresAt.IsZero() {
			updates["git_hub_token_expires_at"] = gothUser.ExpiresAt
		}
	}

	if err := initializers.DB.Model(user).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to link OAuth account: %v", err)
	}

	return user, nil
}

func createNewOAuthUser(gothUser goth.User) (*models.User, error) {
	user := models.User{
		ID:        uuid.New(),
		Email:     gothUser.Email,
		Provider:  &gothUser.Provider,
	}

	switch gothUser.Provider {
	case "google":
		user.GoogleID = &gothUser.UserID
		user.GoogleAccessToken = &gothUser.AccessToken
		if gothUser.RefreshToken != "" {
			user.GoogleRefreshToken = &gothUser.RefreshToken
		}
		if !gothUser.ExpiresAt.IsZero() {
			user.GoogleTokenExpiresAt = &gothUser.ExpiresAt
		}
	case "github":
		user.GitHubID = &gothUser.UserID
		user.GitHubAccessToken = &gothUser.AccessToken
		if !gothUser.ExpiresAt.IsZero() {
			user.GitHubTokenExpiresAt = &gothUser.ExpiresAt
		}
	}

	if err := initializers.DB.Create(&user).Error; err != nil {
		return nil, fmt.Errorf("failed to create user: %v", err)
	}

	return &user, nil
}
