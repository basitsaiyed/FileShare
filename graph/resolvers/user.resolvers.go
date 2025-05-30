package resolvers

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.73

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"golang.org/x/crypto/bcrypt"

	"github.com/basit/fileshare-backend/graph/model"
	"github.com/basit/fileshare-backend/initializers"
	"github.com/basit/fileshare-backend/models"
)

// ChangePassword is the resolver for the changePassword field.
func (r *mutationResolver) ChangePassword(ctx context.Context, currentPassword string, newPassword string) (bool, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return false, err
	}

	var user models.User
	if err := initializers.DB.First(&user, "id = ?", userID).Error; err != nil {
		return false, fmt.Errorf("user not found")
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		return false, fmt.Errorf("incorrect current password")
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return false, fmt.Errorf("failed to hash new password")
	}

	user.PasswordHash = string(newHash)
	if err := initializers.DB.Save(&user).Error; err != nil {
		return false, fmt.Errorf("failed to update password")
	}

	return true, nil
}

// UpdateNotificationPreferences is the resolver for the updateNotificationPreferences field.
func (r *mutationResolver) UpdateNotificationPreferences(ctx context.Context, downloadAlerts bool, expiryReminders bool) (*model.User, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	var user models.User
	if err := initializers.DB.First(&user, "id = ?", userID).Error; err != nil {
		return nil, fmt.Errorf("user not found")
	}

	user.DownloadAlerts = downloadAlerts
	user.ExpiryReminders = expiryReminders

	if err := initializers.DB.Save(&user).Error; err != nil {
		return nil, fmt.Errorf("failed to update preferences")
	}

	return &model.User{
		ID:              user.ID.String(),
		Email:           user.Email,
		CreatedAt:       user.CreatedAt.String(),
		DownloadAlerts:  user.DownloadAlerts,
		ExpiryReminders: user.ExpiryReminders,
	}, nil
}

// DeleteAccount is the resolver for the deleteAccount field.
func (r *mutationResolver) DeleteAccount(ctx context.Context) (bool, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return false, err
	}

	tx := initializers.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Try to delete from S3, but don't fail account deletion if this fails
	s3DeleteErr := deleteUserFilesFromS3(userID.String())
	if s3DeleteErr != nil {
		log.Printf("S3 cleanup failed for user %s: %v", userID.String(), s3DeleteErr)
		// You could queue this for retry later
		// queueS3Cleanup(userID.String())
	}

	// Step 1: Get user's files first
	var userFiles []models.File
	if err := tx.Where("user_id = ?", userID).Find(&userFiles).Error; err != nil {
		tx.Rollback()
		return false, fmt.Errorf("failed to fetch user files: %w", err)
	}

	// Step 2: Extract file IDs
	var fileIDs []string
	for _, file := range userFiles {
		fileIDs = append(fileIDs, file.ID.String())
	}

	// Step 3: Delete download events for those file IDs
	if len(fileIDs) > 0 {
		if err := tx.Where("file_id IN ?", fileIDs).Delete(&models.DownloadEvent{}).Error; err != nil {
			tx.Rollback()
			return false, fmt.Errorf("failed to delete download events: %w", err)
		}
	}

	// Step 4: Delete the files themselves
	if err := tx.Where("user_id = ?", userID).Delete(&models.File{}).Error; err != nil {
		tx.Rollback()
		return false, fmt.Errorf("failed to delete files: %w", err)
	}

	if err := tx.Delete(&models.User{}, "id = ?", userID).Error; err != nil {
		tx.Rollback()
		return false, fmt.Errorf("failed to delete account: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return false, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// If S3 cleanup failed, you might want to return a warning
	if s3DeleteErr != nil {
		log.Printf("Account deleted but S3 cleanup incomplete for user %s", userID.String())
	}

	return true, nil
}

// Me is the resolver for the me field.
func (r *queryResolver) Me(ctx context.Context) (*model.User, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	var user models.User
	if err := initializers.DB.First(&user, "id = ?", userID).Error; err != nil {
		return nil, fmt.Errorf("user not found")
	}

	return &model.User{
		ID:              user.ID.String(),
		Email:           user.Email,
		CreatedAt:       user.CreatedAt.String(),
		DownloadAlerts:  user.DownloadAlerts,
		ExpiryReminders: user.ExpiryReminders,
	}, nil
}

// UserStats is the resolver for the userStats field.
func (r *queryResolver) UserStats(ctx context.Context) (*model.UserStats, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("unauthorized")
	}

	var totalFiles int64
	var totalDownloads int64
	var totalSizeBytes int64

	// Total Files
	if err := initializers.DB.Model(&models.File{}).
		Where("user_id = ?", userID).
		Count(&totalFiles).Error; err != nil {
		return nil, err
	}

	// Total Downloads
	err = initializers.DB.
		Model(&models.DownloadEvent{}).
		Joins("JOIN files ON files.id = download_events.file_id").
		Where("files.user_id = ?", userID).
		Count(&totalDownloads).Error
	if err != nil {
		return nil, err
	}

	// Total Storage Used (in bytes)
	err = initializers.DB.
		Model(&models.File{}).
		Where("user_id = ?", userID).
		Select("COALESCE(SUM(file_size), 0)").Scan(&totalSizeBytes).Error
	if err != nil {
		return nil, err
	}

	return &model.UserStats{
		TotalFiles:     int32(totalFiles),
		TotalDownloads: int32(totalDownloads),
		StorageUsed:    formatSize(totalSizeBytes),
	}, nil
}

func deleteUserFilesFromS3(userID string) error {
	var files []models.File
	if err := initializers.DB.Where("user_id = ?", userID).Find(&files).Error; err != nil {
		return fmt.Errorf("failed to fetch user files: %w", err)
	}

	s3Client := initializers.S3Client
	var deleteErrors []error

	for _, file := range files {
		_, err := s3Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
			Bucket: aws.String(initializers.S3Bucket),
			Key:    aws.String(file.StoragePath), // This is now the S3 key after your fix
		})
		if err != nil {
			log.Printf("Failed to delete S3 object %s: %v", file.StoragePath, err)
			deleteErrors = append(deleteErrors, err)
		}
	}

	if len(deleteErrors) > 0 {
		return fmt.Errorf("failed to delete %d files from S3", len(deleteErrors))
	}

	return nil
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}


// !!! WARNING !!!
// The code below was going to be deleted when updating resolvers. It has been copied here so you have
// one last chance to move it out of harms way if you want. There are two reasons this happens:
//  - When renaming or deleting a resolver the old code will be put in here. You can safely delete
//    it when you're done.
//  - You have helper methods in this file. Move them out to keep these resolver files clean.
/*
	func deleteUserFilesFromS3(userID string) error {
	var files []models.File
	if err := initializers.DB.Where("user_id = ?", userID).Find(&files).Error; err != nil {
		return fmt.Errorf("failed to fetch user files: %w", err)
	}

	s3Client := initializers.S3Client
	var deleteErrors []error

	for _, file := range files {
		_, err := s3Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
			Bucket: aws.String(initializers.S3Bucket),
			Key:    aws.String(file.StoragePath), // This is now the S3 key after your fix
		})
		if err != nil {
			log.Printf("Failed to delete S3 object %s: %v", file.StoragePath, err)
			deleteErrors = append(deleteErrors, err)
		}
	}

	if len(deleteErrors) > 0 {
		return fmt.Errorf("failed to delete %d files from S3", len(deleteErrors))
	}

	return nil
}
*/
