package routes

import (
	"github.com/gin-gonic/gin"

	"github.com/basit/fileshare-backend/auth/Oauth"
	"github.com/basit/fileshare-backend/auth/middleware"
	"github.com/basit/fileshare-backend/handlers"
)

func RegisterFileRoutes(r *gin.Engine) {
	// Public download route (no auth required)
	r.GET("/auth/:provider", Oauth.OauthCallbackHandler)
	r.GET("/auth/:provider/callback", Oauth.CompleteAuth)
	r.GET("/api/files/download/:slug", handlers.DownloadFile)
	r.GET("/d/:slug", handlers.HandlePublicDownload)

	// Protected file management routes (auth required)
	fileGroup := r.Group("/api/files")
	fileGroup.Use(middleware.AuthRequired())
	{
		fileGroup.POST("/upload", handlers.UploadFile)
		fileGroup.GET("/", handlers.ListFiles)
		fileGroup.PUT("/:id/rename", handlers.RenameFile)
		fileGroup.DELETE("/:id", handlers.DeleteFile)
		fileGroup.GET("/:slug/qr", handlers.GetQRCode)
	}
}
