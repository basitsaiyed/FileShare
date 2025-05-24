package jobs

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/basit/fileshare-backend/initializers"
	"github.com/basit/fileshare-backend/models"
	storage_go "github.com/supabase-community/storage-go"
)

func StartCleanupJob() {
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for range ticker.C {
			cleanupExpiredFiles()
		}
	}()
}

func cleanupExpiredFiles() {
	var expiredFiles []models.File

	// Find expired files
	if err := initializers.DB.Where("expires_at < ?", time.Now()).Find(&expiredFiles).Error; err != nil {
		log.Printf("Error finding expired files: %v", err)
		return
	}

	// Delete files from storage and database
	for _, file := range expiredFiles {
		// Delete from Supabase storage
		supaURL := os.Getenv("SUPABASE_URL")
		supaKey := os.Getenv("SUPABASE_KEY")
		storageClient := storage_go.NewClient(supaURL, supaKey, nil)

		result, err := storageClient.RemoveFile("fileshare", []string{file.StoragePath})
		if err != nil {
			log.Printf("Error deleting file from storage: %v", err)
			continue
		}
		fmt.Println("Deleted file from storage:", result)

		// Delete from database
		if err := initializers.DB.Delete(&file).Error; err != nil {
			log.Printf("Error deleting file from database: %v", err)
		}
	}
}
