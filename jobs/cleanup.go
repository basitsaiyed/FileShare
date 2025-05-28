package jobs

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/basit/fileshare-backend/initializers"
	"github.com/basit/fileshare-backend/models"
)

func StartCleanupJob() {
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for range ticker.C {
			cleanupExpiredFiles()
		}
	}()
	log.Println("Cleanup job started - runs every hour")
}

func cleanupExpiredFiles() {
	log.Println("Starting cleanup of expired files...")

	var expiredFiles []models.File

	// Find expired files
	if err := initializers.DB.Where("expires_at < ?", time.Now()).Find(&expiredFiles).Error; err != nil {
		log.Printf("Error finding expired files: %v", err)
		return
	}

	if len(expiredFiles) == 0 {
		log.Println("No expired files found")
		return
	}

	log.Printf("Found %d expired files to cleanup", len(expiredFiles))

	// Delete files from S3 storage and database
	for _, file := range expiredFiles {
		// Delete from S3
		if err := deleteFileFromS3(file.StoragePath); err != nil {
			log.Printf("Error deleting file %s from S3: %v", file.StoragePath, err)
			continue // Skip database deletion if S3 deletion fails
		}

		// Delete associated download events first (foreign key constraint)
		if err := initializers.DB.Where("file_id = ?", file.ID).Delete(&models.DownloadEvent{}).Error; err != nil {
			log.Printf("Error deleting download events for file %s: %v", file.ID, err)
		}

		// Delete from database
		if err := initializers.DB.Delete(&file).Error; err != nil {
			log.Printf("Error deleting file %s from database: %v", file.ID, err)
		} else {
			log.Printf("Successfully cleaned up expired file: %s (Original: %s)", file.ID, file.OriginalName)
		}
	}

	log.Printf("Cleanup completed. Processed %d expired files", len(expiredFiles))
}

// deleteFileFromS3 deletes a single file from S3 storage
func deleteFileFromS3(key string) error {
	s3Client := initializers.S3Client

	_, err := s3Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(getBucketName()),
		Key:    aws.String(key),
	})

	if err != nil {
		return fmt.Errorf("failed to delete S3 object %s: %w", key, err)
	}

	log.Printf("Successfully deleted file from S3: %s", key)
	return nil
}

// getBucketName returns the S3 bucket name from environment or initializers
func getBucketName() string {
	// Try environment variable first (consistent with your DeleteFile handler)
	if bucketName := os.Getenv("AWS_BUCKET_NAME"); bucketName != "" {
		return bucketName
	}
	// Fallback to initializers (consistent with your upload function)
	return initializers.S3Bucket
}

// Optional: Cleanup job with configurable interval
func StartCleanupJobWithInterval(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		// Run cleanup immediately on start
		cleanupExpiredFiles()

		// Then run on schedule
		for range ticker.C {
			cleanupExpiredFiles()
		}
	}()
	log.Printf("Cleanup job started - runs every %v", interval)
}

// Optional: Manual cleanup trigger for admin endpoints
func RunCleanupNow() error {
	log.Println("Manual cleanup triggered")
	cleanupExpiredFiles()
	return nil
}
