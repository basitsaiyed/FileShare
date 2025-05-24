package initializers

import (
	"log"
	"os"

	"github.com/basit/fileshare-backend/models"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func ConnectToDatabase() {
	if os.Getenv("RENDER") == "" {
		if err := godotenv.Load(); err != nil {
			log.Println("⚠️  Warning: No .env file found. Using system environment variables.")
		}
	}
	dsn := os.Getenv("DB_URL")
	if dsn == "" {
		log.Fatal("❌ DB_URL is not set in environment variables")
	}
	var err error

	DB, err = gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})

	// DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Failed to connect to the database: %v", err)
	}
	if DB == nil {
		log.Fatal("❌ Database connection returned nil")
	}
	// sqlDB, err := DB.DB()
	// if err != nil {
	// 	log.Fatalf("Failed to get underlying SQL DB instance: %v", err)
	// }

	// Set the connection pool parameters
	// sqlDB.SetMaxOpenConns(4)                   // Set the maximum number of open connections
	// sqlDB.SetMaxIdleConns(2)                   // Set the maximum number of idle connections
	// sqlDB.SetConnMaxLifetime(30 * time.Minute) // Set the maximum lifetime of a connection

	// DB.AutoMigrate(&models.Activity{})
	if err := DB.AutoMigrate(
		&models.User{},
		&models.File{},
	); err != nil {
		log.Fatalf("❌ Failed to migrate database schema: %v", err)
	}
	log.Println("✅ Database connected and migrated successfully")
}
