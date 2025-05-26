package routes

import (
	"github.com/gin-gonic/gin"

	"github.com/basit/fileshare-backend/auth/middleware"
	"github.com/basit/fileshare-backend/handlers"

)

func RegisterFileRoutes(r *gin.Engine) {
	// Public download route (no auth required)
	r.GET("/api/files/download/:slug", handlers.DownloadFile)

	// Protected file management routes (auth required)
	fileGroup := r.Group("/api/files")
	fileGroup.Use(middleware.AuthRequired())
	{
		fileGroup.POST("/upload", handlers.UploadFile)
		fileGroup.GET("/", handlers.ListFiles)
		fileGroup.PUT("/:id/rename", handlers.RenameFile)
		fileGroup.DELETE("/:id", handlers.DeleteFile)
	}
}

