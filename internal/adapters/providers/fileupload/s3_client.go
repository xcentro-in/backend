package fileupload

import (
	"context"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
	"xcentro.in/internal/shared/config"
)

// GenerateUploadKey generates a unique key for uploads with the given directory prefix
func GenerateUploadKey(directory string, filename string) string {
	ext := filepath.Ext(filename)
	return fmt.Sprintf("%s/%s/%s%s",
		directory,
		time.Now().Format("2006/01/02"),
		uuid.Must(uuid.NewV7()).String(),
		ext,
	)
}

// GeneratePIIKey generates a unique key for PII uploads (for backwards compatibility)
func GeneratePIIKey(filename string) string {
	return GenerateUploadKey("PII", filename)
}

// GenerateUploadsKey generates a unique key for general uploads (for backwards compatibility)
func GenerateUploadsKey(filename string) string {
	return GenerateUploadKey("uploads", filename)
}

// S3Client implements the FileUploader interface for AWS S3
type S3Client struct {
	client *s3.Client
	bucket string
}

// NewS3Client creates a new S3 client
func NewS3Client(bucket string) (*S3Client, error) {
	// Backward compatible simple constructor: relies on ambient AWS_* env or instance profile.
	// Region must be resolvable via env/shared config; optionally override via AWS_REGION env.
	awsCfg, err := awsconfig.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS SDK config: %w", err)
	}
	return &S3Client{client: s3.NewFromConfig(awsCfg), bucket: bucket}, nil
}

// NewS3ClientFromConfig builds S3 client using structured application config.
// Supports optional custom endpoint (MinIO/localstack) and static credentials override.
func NewS3ClientFromConfig(c config.S3Config) (*S3Client, error) {
	if c.Bucket == "" {
		return nil, fmt.Errorf("s3 bucket is required")
	}
	if c.Region == "" {
		return nil, fmt.Errorf("s3 region is required")
	}

	loadOpts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(c.Region)}

	if c.Profile != "" {
		loadOpts = append(loadOpts, awsconfig.WithSharedConfigProfile(c.Profile))
	}

	if c.Endpoint != "" {
		// Custom endpoint (e.g., MinIO/localstack)
		loadOpts = append(loadOpts, awsconfig.WithBaseEndpoint(c.Endpoint))
	}

	// If explicit static credentials provided, override chain
	if c.AccessKey != "" && c.SecretKey != "" {
		staticCreds := credentials.NewStaticCredentialsProvider(c.AccessKey, c.SecretKey, c.SessionToken)
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(staticCreds))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.TODO(), loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	// Allow fallback if region not set in config but present in env (defensive)
	if awsCfg.Region == "" {
		if envRegion := os.Getenv("AWS_REGION"); envRegion != "" {
			awsCfg.Region = envRegion
		}
	}
	if awsCfg.Region == "" { // still empty
		return nil, fmt.Errorf("aws region could not be resolved")
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if c.UsePathStyle {
			o.UsePathStyle = true
		}
	})

	return &S3Client{client: s3Client, bucket: c.Bucket}, nil
}

// UploadFile streams file directly to S3 (server-side upload)
func (s *S3Client) UploadFile(ctx context.Context, file *multipart.FileHeader, key string) (string, error) {
	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer src.Close()

	// Use provided key, append extension if not already present
	ext := filepath.Ext(file.Filename)
	if ext != "" && filepath.Ext(key) == "" {
		key = key + ext
	}

	// Detect content type
	contentType := file.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Upload directly from multipart file reader
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        src,
		ContentType: aws.String(contentType),
	})

	if err != nil {
		return "", fmt.Errorf("failed to upload to S3: %w", err)
	}

	return key, nil
}

// GetPresignedUploadURL generates a presigned URL for CLIENT-SIDE upload
func (s *S3Client) GetPresignedUploadURL(ctx context.Context, contentType string, key string, expiration time.Duration) (string, string, error) {
	presignClient := s3.NewPresignClient(s.client)

	req, err := presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expiration
	})

	if err != nil {
		return "", "", fmt.Errorf("failed to generate presigned upload URL: %w", err)
	}

	return req.URL, key, nil
}

// GetPresignedDownloadURL generates a presigned URL for downloading
func (s *S3Client) GetPresignedDownloadURL(ctx context.Context, key string, expiration time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(s.client)

	req, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expiration
	})

	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return req.URL, nil
}

// GenerateMultipartUploadURLs creates presigned URLs for multipart upload of large files.
// parts indicates how many parts the client intends to upload. Caller must later
// CompleteMultipartUpload using the AWS SDK or backend endpoint (not handled here).
func (s *S3Client) GenerateMultipartUploadURLs(ctx context.Context, filename string, contentType string, parts int) (map[string]interface{}, error) {
	if parts <= 0 {
		return nil, fmt.Errorf("parts must be > 0")
	}

	ext := filepath.Ext(filename)
	key := fmt.Sprintf("uploads/%s/%s%s",
		time.Now().Format("2006/01/02"),
		uuid.Must(uuid.NewV7()).String(),
		ext,
	)

	// Initiate multipart upload
	multipartResp, err := s.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create multipart upload: %w", err)
	}

	presignClient := s3.NewPresignClient(s.client)
	partURLs := make([]string, parts)

	for i := 0; i < parts; i++ {
		partNumber := int32(i + 1)
		req, err := presignClient.PresignUploadPart(ctx, &s3.UploadPartInput{
			Bucket:     aws.String(s.bucket),
			Key:        aws.String(key),
			PartNumber: aws.Int32(partNumber),
			UploadId:   multipartResp.UploadId,
		}, func(opts *s3.PresignOptions) {
			opts.Expires = 60 * time.Minute
		})
		if err != nil {
			return nil, fmt.Errorf("failed to generate presigned URL for part %d: %w", partNumber, err)
		}
		partURLs[i] = req.URL
	}

	return map[string]interface{}{
		"upload_id": *multipartResp.UploadId,
		"key":       key,
		"part_urls": partURLs,
	}, nil
}

// CompleteMultipartUpload finalizes a multipart upload after all parts are uploaded.
func (s *S3Client) CompleteMultipartUpload(ctx context.Context, key string, uploadID string, parts []MultipartUploadedPart) error {
	if uploadID == "" || key == "" {
		return fmt.Errorf("key and uploadID are required")
	}
	if len(parts) == 0 {
		return fmt.Errorf("no parts provided")
	}

	completedParts := make([]s3types.CompletedPart, len(parts))
	for i, p := range parts {
		if p.ETag == "" || p.PartNumber <= 0 {
			return fmt.Errorf("invalid part: %#v", p)
		}
		completedParts[i] = s3types.CompletedPart{
			ETag:       aws.String(p.ETag),
			PartNumber: aws.Int32(int32(p.PartNumber)),
		}
	}

	_, err := s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &s3types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to complete multipart upload: %w", err)
	}
	return nil
}

// DeleteObject removes an object from S3.
func (s *S3Client) DeleteObject(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("key is required")
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete S3 object: %w", err)
	}
	return nil
}

// ConfirmUpload verifies that the object exists and returns a presigned download URL.
func (s *S3Client) ConfirmUpload(ctx context.Context, key string, expiration time.Duration) (string, error) {
	if key == "" {
		return "", fmt.Errorf("key is required")
	}
	// HeadObject to verify existence
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return "", fmt.Errorf("file not found or inaccessible: %w", err)
	}
	return s.GetPresignedDownloadURL(ctx, key, expiration)
}
