package initializers

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var S3Client *s3.Client
var S3Bucket string

func InitAWS() {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(os.Getenv("AWS_REGION")),
	)
	if err != nil {
		log.Fatalf("unable to load AWS SDK config: %v", err)
	}

	S3Client = s3.NewFromConfig(cfg)
	S3Bucket = os.Getenv("AWS_BUCKET_NAME")
}
