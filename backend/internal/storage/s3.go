package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

type S3 struct {
	S3     *s3.Client
	Bucket string
}

func isDev() bool {
	return os.Getenv("APP_ENV") == "development"
}

func bucketName() string {
	if isDev() {
		if b := os.Getenv("DEV_S3_BUCKET"); b != "" {
			return b
		}
	}
	if b := os.Getenv("S3_BUCKET"); b != "" {
		return b
	}
	return "expressions-india"
}

func InitS3() *S3 {
	var cfg aws.Config
	var err error

	region := os.Getenv("S3_REGION")
	if region == "" {
		region = "ap-south-1"
	}

	// In development, DEV_S3_URL acts as the Minio endpoint.
	endpoint := os.Getenv("S3_ENDPOINT")
	if isDev() && endpoint == "" {
		endpoint = os.Getenv("DEV_S3_URL")
	}

	if endpoint != "" {
		// Local dev / Minio: use static credentials.
		username := os.Getenv("S3_USERNAME")
		password := os.Getenv("S3_PASSWORD")
		if isDev() {
			if u := os.Getenv("DEV_S3_USERNAME"); u != "" {
				username = u
			}
			if p := os.Getenv("DEV_S3_PASSWORD"); p != "" {
				password = p
			}
		}
		cfg, err = config.LoadDefaultConfig(context.TODO(),
			config.WithRegion(region),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				username, password, os.Getenv("S3_SESSION"),
			)),
		)
	} else {
		// Real AWS: SDK picks up AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY from env.
		cfg, err = config.LoadDefaultConfig(context.TODO(),
			config.WithRegion(region),
		)
	}

	if err != nil {
		log.Fatal("Failed to initialize s3.")
		return nil
	}

	usePathStyle := endpoint != ""
	return &S3{
		Bucket: bucketName(),
		S3: s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.UsePathStyle = usePathStyle
			if endpoint != "" {
				o.BaseEndpoint = aws.String(endpoint)
			}
		}),
	}
}

func (s *S3) UploadLocal(fileUrl string) (string, string, error) {
	uploader := manager.NewUploader(s.S3, func(u *manager.Uploader) {
		u.PartSize = 5 * 1024 * 1024
		u.LeavePartsOnError = false
	})
	file, err := os.Open(fileUrl)
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	contentType := mime.TypeByExtension(filepath.Ext(fileUrl))
	if contentType == "" {
		buffer := make([]byte, 512)
		n, _ := file.Read(buffer)
		contentType = http.DetectContentType(buffer[:n])
	}
	if _, err := file.Seek(0, 0); err != nil {
		return "", "", fmt.Errorf("seek failed: %w", err)
	}

	s3Key := uuid.Must(uuid.NewV7()).String()
	_, err = uploader.Upload(context.TODO(), &s3.PutObjectInput{
		Bucket:             aws.String(s.Bucket),
		Key:                aws.String(s3Key),
		Body:               file,
		ContentType:        aws.String(contentType),
		ContentDisposition: aws.String("inline"),
	})
	if err != nil {
		return "", "", err
	}
	return s.PublicURL(s3Key), s3Key, nil
}
func (s *S3) UploadNetwork(file io.Reader) (string, string, string, error) {

	uploader := manager.NewUploader(s.S3)

	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", "", "", fmt.Errorf("failed to read file header: %w", err)
	}
	contentType := http.DetectContentType(buffer[:n])

	fullBody := io.MultiReader(bytes.NewReader(buffer[:n]), file)

	s3Key := uuid.Must(uuid.NewV7()).String()
	_, err = uploader.Upload(context.TODO(), &s3.PutObjectInput{
		Bucket:             aws.String(s.Bucket),
		Key:                aws.String(s3Key),
		Body:               fullBody,
		ContentType:        aws.String(contentType),
		ContentDisposition: aws.String("inline"),
	})

	if err != nil {
		return "", "", "", fmt.Errorf("s3 upload failed: %w", err)
	}

	return s.PublicURL(s3Key), s3Key, contentType, nil
}

func (s *S3) DeleteFromS3(s3Key string) error {

	_, err := s.S3.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(s3Key),
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *S3) Delete(s3Key string) error {
	if err := s.DeleteFromS3(s3Key); err != nil {
		return err
	}
	return nil
}

// PresignUpload generates a presigned PUT URL for the given object key and
// content type. The URL expires after ttl and the client must supply a
// matching Content-Type header when uploading.
func (s *S3) PresignUpload(id, contentType string, ttl time.Duration) (presignedURL string, err error) {
	pc := s3.NewPresignClient(s.S3)
	req, err := pc.PresignPutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(s.Bucket),
		Key:         aws.String(id),
		ContentType: aws.String(contentType),
	}, func(o *s3.PresignOptions) {
		o.Expires = ttl
	})
	if err != nil {
		return "", fmt.Errorf("presign failed: %w", err)
	}
	return req.URL, nil
}

// PublicURL returns the publicly accessible URL for an already-uploaded object.
// For real AWS the URL is served via CloudFront: https://<domain>/<key>.
// For Minio the URL includes the bucket: http://<host>/<bucket>/<key>.
func (s *S3) PublicURL(id string) string {
	if isDev() {
		base := strings.TrimRight(os.Getenv("DEV_S3_URL"), "/")
		return fmt.Sprintf("%s/%s/%s", base, s.Bucket, id)
	}
	base := strings.TrimRight(os.Getenv("S3_URL"), "/")
	if os.Getenv("S3_ENDPOINT") != "" {
		return fmt.Sprintf("%s/%s/%s", base, s.Bucket, id)
	}
	return fmt.Sprintf("%s/%s", base, id)
}
