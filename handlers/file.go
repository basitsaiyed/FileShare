package handlers

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lithammer/shortuuid/v4"
	"github.com/skip2/go-qrcode"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/basit/fileshare-backend/initializers"
	"github.com/basit/fileshare-backend/models"

)

// uploadFileToS3 uploads a file to AWS S3
func uploadFileToS3(file multipart.File, fileHeader *multipart.FileHeader, key string) (string, error) {
	uploader := manager.NewUploader(initializers.S3Client)

	buffer := bytes.NewBuffer(nil)
	if _, err := buffer.ReadFrom(file); err != nil {
		return "", err
	}

	_, err := uploader.Upload(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(initializers.S3Bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(buffer.Bytes()),
		ContentType: aws.String(fileHeader.Header.Get("Content-Type")),
		// ACL:         types.ObjectCannedACLPublicRead, // or "private" if not public
	})

	if err != nil {
		log.Printf("S3 Upload Error: %v\n", err)
		return "", err
	}

	url := "https://" + initializers.S3Bucket + ".s3." + os.Getenv("AWS_REGION") + ".amazonaws.com/" + key
	return url, nil
}

func UploadFile(c *gin.Context) {
	userID := c.MustGet("userID").(uuid.UUID)

	baseURL := os.Getenv("BASE_URL")

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
		return
	}

	key := uuid.New().String() + "_" + fileHeader.Filename
	s3URL, err := uploadFileToS3(file, fileHeader, key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload to S3"})
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

	// uploadPath := "uploads/" + uuid.New().String() + "_" + fileHeader.Filename

	// // Save to local disk
	// if err := c.SaveUploadedFile(file, uploadPath); err != nil {
	// 	c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
	// 	return
	// }
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 days from now

	downloadSlug := generateSlug()

	mimeType := mime.TypeByExtension(filepath.Ext(fileHeader.Filename))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	// Save metadata in DB
	newFile := models.File{
		ID:           uuid.New(),
		OriginalName: fileHeader.Filename,
		StoragePath:  key,
		FileSize:     int32(fileHeader.Size),
		DownloadSlug: downloadSlug,
		CreatedAt:    time.Now(),
		UserID:       &userID,
		ExpiresAt:    &expiresAt,
		PublicURL:    fmt.Sprintf("%s/d/%s", baseURL, downloadSlug),
		IsPublic:     true,
		ContentType:  mimeType,
	}

	if err := initializers.DB.Create(&newFile).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "DB save failed"})
		return
	}
	initializers.DB.Preload("User").First(&newFile, "id = ?", newFile.ID)

	c.JSON(http.StatusOK, gin.H{
		"file":   newFile,
		"s3_url": s3URL,
		"qr_url": newFile.QRCodePath,
	})
}

// Helper function to generate full S3 URL from key
func generateS3URL(key string) string {
	return "https://" + initializers.S3Bucket + ".s3." + os.Getenv("AWS_REGION") + ".amazonaws.com/" + key
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

	// Add full URLs to each file for frontend convenience
	type FileWithURL struct {
		models.File
		URL          string `json:"url"`
		ShareableURL string `json:"shareableUrl"`
	}

	var filesWithURLs []FileWithURL
	for _, file := range files {
		filesWithURLs = append(filesWithURLs, FileWithURL{
			File:         file,
			URL:          generateS3URL(file.StoragePath),
			ShareableURL: file.PublicURL,
		})
	}

	c.JSON(http.StatusOK, gin.H{"files": filesWithURLs})
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

	// Fetch file from DB first to get StoragePath
	var file models.File
	if err := initializers.DB.First(&file, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	s3Client := initializers.S3Client
	_, err := s3Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(os.Getenv("AWS_BUCKET_NAME")),
		Key:    aws.String(file.StoragePath), // This must be the S3 object key
	})
	if err != nil {
		log.Printf("S3 Deletion Error: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete from S3"})
		return
	}

	if err := initializers.DB.Where("file_id = ?", file.ID).Delete(&models.DownloadEvent{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete download events"})
		return
	}

	if err := initializers.DB.Delete(&file).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete from DB"})
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
	preview := c.Query("preview") == "true"

	var file models.File

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
	if file.ExpiresAt != nil && time.Now().After(*file.ExpiresAt) {
		c.JSON(http.StatusGone, gin.H{"error": "This file has expired"})
		return
	}

	// Atomic counter update
	initializers.DB.Model(&file).UpdateColumn("download_count", gorm.Expr("download_count + ?", 1))
	initializers.DB.Model(&file).Update("last_downloaded_at", time.Now())

	// Log the download event
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

	s3URL, err := generatePresignedURL(file.StoragePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate S3 URL"})
		return
	}

	resp, err := http.Get(s3URL)
	if err != nil || resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch file"})
		return
	}
	defer resp.Body.Close()

	// ⬇️ Only force download if not previewing
	if !preview {
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", file.OriginalName))
	}

	c.DataFromReader(http.StatusOK, resp.ContentLength, resp.Header.Get("Content-Type"), resp.Body, nil)
}

func HandlePublicDownload(c *gin.Context) {
	slug := c.Param("slug")

	var file models.File
	if err := initializers.DB.First(&file, "download_slug = ?", slug).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	if !file.IsPublic {
		c.JSON(http.StatusForbidden, gin.H{"error": "This file is not public"})
		return
	}

	// Redirect internally to the actual download endpoint
	c.Redirect(http.StatusTemporaryRedirect, "/api/files/download/"+slug+"?"+c.Request.URL.RawQuery)
}

func generatePresignedURL(key string) (string, error) {
	client := initializers.S3Client
	presigner := s3.NewPresignClient(client)

	req, err := presigner.PresignGetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(os.Getenv("AWS_BUCKET_NAME")),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(15*time.Minute)) // URL valid for 15 mins

	if err != nil {
		return "", err
	}

	return req.URL, nil
}

func GetQRCode(c *gin.Context) {
	slug := c.Param("slug")

	// Optional: Validate if the file exists
	var file models.File
	if err := initializers.DB.Where("download_slug = ?", slug).First(&file).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}
	fmt.Println("Base URL:", os.Getenv("QR_BASE_URL"))

	url := fmt.Sprintf("%s/d/%s", os.Getenv("QR_BASE_URL"), slug)

	png, err := qrcode.Encode(url, qrcode.Medium, 256)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate QR code"})
		return
	}

	c.Data(http.StatusOK, "image/png", png)
}
