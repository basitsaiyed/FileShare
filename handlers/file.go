package handlers

import (
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lithammer/shortuuid/v4"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/basit/fileshare-backend/initializers"
	"github.com/basit/fileshare-backend/models"
)

func UploadFile(c *gin.Context) {
	userID := c.MustGet("userID").(uuid.UUID)

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}

	password := c.PostForm("password")
	if password != "" {
		hashBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
			return
		}
		_ = string(hashBytes) // hash is generated but not used
	}

	uploadPath := "uploads/" + uuid.New().String() + "_" + file.Filename

	// Save to local disk
	if err := c.SaveUploadedFile(file, uploadPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 days from now

	// Save metadata in DB
	newFile := models.File{
		ID:           uuid.New(),
		OriginalName: file.Filename,
		StoragePath:  uploadPath,
		FileSize:     int32(file.Size),
		DownloadSlug: generateSlug(),
		CreatedAt:    time.Now(),
		UserID:       &userID,
		ExpiresAt:    &expiresAt,
	}

	if err := initializers.DB.Create(&newFile).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "DB save failed"})
		return
	}
	initializers.DB.Preload("User").First(&newFile, "id = ?", newFile.ID)

	c.JSON(http.StatusOK, gin.H{"file": newFile})
}

func ListFiles(c *gin.Context) {
	userID := c.MustGet("userID").(uuid.UUID)
	var files []models.File

	if err := initializers.DB.
		Preload("User").
		Where("user_id = ?", userID).
		Find(&files).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch files"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"files": files})
}

func RenameFile(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		NewName string `json:"newName"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.NewName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	if err := initializers.DB.Model(&models.File{}).
		Where("id = ?", id).
		Update("original_name", body.NewName).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Rename failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func DeleteFile(c *gin.Context) {
	id := c.Param("id")

	// Optionally delete from disk too
	var file models.File
	if err := initializers.DB.First(&file, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}
	os.Remove(file.StoragePath)

	if err := initializers.DB.Delete(&file).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Delete failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// generateSlug generates a random slug for file downloads.
func generateSlug() string {
	return shortuuid.New()
}

func DownloadFile(c *gin.Context) {
	slug := c.Param("slug")
	var file models.File
	if file.ExpiresAt != nil && time.Now().After(*file.ExpiresAt) {
		c.JSON(http.StatusGone, gin.H{"error": "This file has expired"})
		return
	}

	if err := initializers.DB.Where("download_slug = ?", slug).First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	if !file.IsPublic {
		userID, exists := c.Get("userID")
		if !exists || file.UserID == nil || *file.UserID != userID.(uuid.UUID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
			return
		}
	}
	if file.PasswordHash != nil {
		var body struct {
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.Password == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Password required"})
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(*file.PasswordHash), []byte(body.Password)); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "Incorrect password"})
			return
		}
	}

	// ✅ Atomic counter update
	initializers.DB.Model(&file).UpdateColumn("download_count", gorm.Expr("download_count + ?", 1))
	initializers.DB.Model(&file).Update("last_downloaded_at", time.Now())

	// ✅ Log the download event
	var userID *uuid.UUID
	if uid, ok := c.Get("userID"); ok {
		uidVal := uid.(uuid.UUID)
		userID = &uidVal
	}

	downloadEvent := models.DownloadEvent{
		ID:        uuid.New(),
		FileID:    file.ID,
		UserID:    userID,
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		CreatedAt: time.Now(),
	}
	initializers.DB.Create(&downloadEvent)

	// ✅ Serve the file
	c.File(file.StoragePath)
}
