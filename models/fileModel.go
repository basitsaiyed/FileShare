package models

import (
	"time"

	"github.com/google/uuid"

)

type File struct {
	ID           uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OriginalName string
	StoragePath  string
	FileSize     int32
	DownloadSlug string    `gorm:"uniqueIndex"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
	ExpiresAt    *time.Time
	PublicURL    string `gorm:"default:null;text;"`
	ContentType  string
	IsPublic     bool `gorm:"default:true"`
	PasswordHash *string `gorm:"default:null"`

	UserID *uuid.UUID
	User   User `gorm:"foreignKey:UserID"`

	DownloadCount    int
	LastDownloadedAt *time.Time

	QRCodePath string `gorm:"default:null"`
}
