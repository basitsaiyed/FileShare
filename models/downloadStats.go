// models/download_event.go
package models

import (
	"time"

	"github.com/google/uuid"
)

type DownloadEvent struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	FileID    uuid.UUID
	File      File `gorm:"foreignKey:FileID"`
	IPAddress string
	UserAgent string
	UserID    *uuid.UUID
	CreatedAt time.Time
}
