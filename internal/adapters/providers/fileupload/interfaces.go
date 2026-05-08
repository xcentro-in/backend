package fileupload

import (
	"context"
	"mime/multipart"
	"time"
)

// FileUploader defines the interface for file upload operations
type FileUploader interface {
	// UploadFile uploads a file directly to storage (server-side upload)
	UploadFile(ctx context.Context, file *multipart.FileHeader, key string) (string, error)

	// GetPresignedUploadURL generates a presigned URL for client-side upload
	GetPresignedUploadURL(ctx context.Context, contentType string, key string, expiration time.Duration) (string, string, error)

	// GetPresignedDownloadURL generates a presigned URL for downloading
	GetPresignedDownloadURL(ctx context.Context, key string, expiration time.Duration) (string, error)

	// GenerateMultipartUploadURLs initiates a multipart upload and returns presigned URLs for each part
	GenerateMultipartUploadURLs(ctx context.Context, filename string, contentType string, parts int) (map[string]interface{}, error)

	// CompleteMultipartUpload completes an initiated multipart upload with the provided parts (ETags)
	CompleteMultipartUpload(ctx context.Context, key string, uploadID string, parts []MultipartUploadedPart) error

	// DeleteObject removes an object from storage
	DeleteObject(ctx context.Context, key string) error

	// ConfirmUpload verifies that the object exists and returns a presigned download URL
	ConfirmUpload(ctx context.Context, key string, expiration time.Duration) (string, error)
}

// UploadResult represents the result of a file upload operation
type UploadResult struct {
	Key         string `json:"key"`
	DownloadURL string `json:"download_url,omitempty"`
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
}

// PresignedUploadResponse represents the response for presigned upload URL request
type PresignedUploadResponse struct {
	UploadURL string `json:"upload_url"`
	Key       string `json:"key"`
	ExpiresIn int    `json:"expires_in"`
	Method    string `json:"method"`
}

// MultipartUploadedPart represents a single uploaded part (client-provided ETag and part number)
type MultipartUploadedPart struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
}
