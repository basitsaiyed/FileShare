package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID              uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	Email           string    `gorm:"unique;not null"`
	PasswordHash    string    `gorm:"not null"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DownloadAlerts  bool `gorm:"default:true"`
	ExpiryReminders bool `gorm:"default:true"`

	GoogleID           *string `gorm:"uniqueIndex" json:"google_id,omitempty"`
	GitHubID           *string `gorm:"uniqueIndex" json:"github_id,omitempty"`
	GoogleAccessToken  *string `json:"-"` // Don't expose in JSON
	GoogleRefreshToken *string `json:"-"`
	GitHubAccessToken  *string `json:"-"`
	Provider           *string `json:"provider,omitempty"`

	GoogleTokenExpiresAt *time.Time `json:"-"`
	GitHubTokenExpiresAt *time.Time `json:"-"`
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}
