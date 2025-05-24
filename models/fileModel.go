package models

import (
	"time"

	"github.com/google/uuid"
)

type File struct {
	ID           uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	UserID       *uuid.UUID
	OriginalName string
	StoragePath  string
	FileSize     int32
	ContentType  string
	DownloadSlug string `gorm:"uniqueIndex"`
	IsPublic     bool   `gorm:"default:true"`
	ExpiresAt    *time.Time
	CreatedAt    time.Time
	PublicURL	string `gorm:"default:null"`

	User User `gorm:"foreignKey:UserID"`
}
