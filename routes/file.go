package routes

import (
	"github.com/gin-gonic/gin"

	"github.com/basit/fileshare-backend/auth/middleware"
	"github.com/basit/fileshare-backend/handlers"

)

func RegisterFileRoutes(r *gin.Engine) {
	fileGroup := r.Group("/api/files")
	fileGroup.GET("/download/:slug", handlers.DownloadFile) // public download
	fileGroup.Use(middleware.AuthRequired()) // protect all file endpoints

	fileGroup.POST("/upload", handlers.UploadFile)
	fileGroup.GET("/", handlers.ListFiles)
	fileGroup.PUT("/:id/rename", handlers.RenameFile)
	fileGroup.DELETE("/:id", handlers.DeleteFile)
}
