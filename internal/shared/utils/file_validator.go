package utils

import (
	"fmt"
	"io"
	"mime/multipart"

	"github.com/gabriel-vasile/mimetype"
)

// Allowed MIME types for CSV uploads
var allowedMIMETypes = map[string]bool{
	"text/csv":                                    true, // Standard CSV
	"text/plain":                                  true, // Plain text CSV
	"application/csv":                             true, // Alternative CSV MIME
	"application/vnd.ms-excel":                    true, // CSV exported from Excel
	"text/comma-separated-values":                 true, // Alternative CSV MIME
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": false, // XLSX - future support
	"application/vnd.ms-excel.sheet.macroEnabled.12":                     false, // XLSM - not allowed (macros)
}

// Maximum file size in bytes (5 MB)
const MaxFileSize = 5 * 1024 * 1024

// FileValidationError represents a file validation error
type FileValidationError struct {
	Code    string
	Message string
}

func (e *FileValidationError) Error() string {
	return e.Message
}

// ValidateUploadedFile performs comprehensive validation on uploaded file
// Checks: file size, magic bytes (MIME type), and file extension
func ValidateUploadedFile(file *multipart.FileHeader) error {
	// Check 1: File size validation
	if file.Size > MaxFileSize {
		return &FileValidationError{
			Code:    "FILE_TOO_LARGE",
			Message: fmt.Sprintf("File size exceeds maximum allowed size of %d MB", MaxFileSize/(1024*1024)),
		}
	}

	// Check minimum file size (at least header row)
	if file.Size < 10 {
		return &FileValidationError{
			Code:    "FILE_TOO_SMALL",
			Message: "File is too small to be a valid CSV",
		}
	}

	// Check 2: Magic byte validation (read first 512 bytes for MIME detection)
	src, err := file.Open()
	if err != nil {
		return &FileValidationError{
			Code:    "FILE_OPEN_ERROR",
			Message: "Failed to open uploaded file for validation",
		}
	}
	defer src.Close()

	// Detect MIME type from content (magic bytes)
	mtype, err := mimetype.DetectReader(src)
	if err != nil {
		return &FileValidationError{
			Code:    "MIME_DETECTION_ERROR",
			Message: "Failed to detect file type",
		}
	}

	// Reset file pointer to beginning for subsequent reads
	if seeker, ok := src.(io.Seeker); ok {
		_, err = seeker.Seek(0, io.SeekStart)
		if err != nil {
			return &FileValidationError{
				Code:    "FILE_SEEK_ERROR",
				Message: "Failed to reset file pointer",
			}
		}
	}

	// Check if detected MIME type is allowed
	detectedMIME := mtype.String()
	allowed, exists := allowedMIMETypes[detectedMIME]
	if !exists {
		// Check if it's a text-based format (fallback)
		if !isTextBasedMIME(detectedMIME) {
			return &FileValidationError{
				Code:    "INVALID_FILE_TYPE",
				Message: fmt.Sprintf("File type not allowed. Detected: %s. Only CSV files are supported.", detectedMIME),
			}
		}
	} else if !allowed {
		return &FileValidationError{
			Code:    "UNSUPPORTED_FILE_TYPE",
			Message: fmt.Sprintf("File type not currently supported: %s", detectedMIME),
		}
	}

	// Check 3: Verify it's not a binary executable or archive
	if isBinaryExecutable(detectedMIME) {
		return &FileValidationError{
			Code:    "EXECUTABLE_NOT_ALLOWED",
			Message: "Executable files and scripts are not allowed",
		}
	}

	if isArchive(detectedMIME) {
		return &FileValidationError{
			Code:    "ARCHIVE_NOT_ALLOWED",
			Message: "Archive files (ZIP, RAR, TAR, etc.) are not allowed",
		}
	}

	// Check 4: Validate file extension matches expected format
	if !hasValidCSVExtension(file.Filename) {
		return &FileValidationError{
			Code:    "INVALID_FILE_EXTENSION",
			Message: "File must have .csv extension",
		}
	}

	return nil
}

// isTextBasedMIME checks if the MIME type is text-based
func isTextBasedMIME(mimeType string) bool {
	textBasedPrefixes := []string{
		"text/",
		"application/csv",
	}

	for _, prefix := range textBasedPrefixes {
		if len(mimeType) >= len(prefix) && mimeType[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// isBinaryExecutable checks if MIME type indicates executable or script
func isBinaryExecutable(mimeType string) bool {
	dangerousMIMEs := []string{
		"application/x-executable",
		"application/x-msdos-program",
		"application/x-msdownload",
		"application/x-elf",
		"application/x-sharedlib",
		"application/x-sh",
		"application/x-shellscript",
		"application/x-bash",
		"application/x-python",
		"application/x-perl",
		"application/x-ruby",
		"application/x-php",
		"application/javascript",
		"application/x-javascript",
		"text/x-python",
		"text/x-php",
		"text/x-shellscript",
	}

	for _, dangerous := range dangerousMIMEs {
		if mimeType == dangerous {
			return true
		}
	}
	return false
}

// isArchive checks if MIME type indicates archive/compressed file
func isArchive(mimeType string) bool {
	archiveMIMEs := []string{
		"application/zip",
		"application/x-zip-compressed",
		"application/x-rar-compressed",
		"application/x-rar",
		"application/x-tar",
		"application/x-gzip",
		"application/gzip",
		"application/x-bzip2",
		"application/x-7z-compressed",
		"application/x-compress",
		"application/x-compressed",
	}

	for _, archiveMIME := range archiveMIMEs {
		if mimeType == archiveMIME {
			return true
		}
	}
	return false
}

// hasValidCSVExtension checks if filename has valid CSV extension
func hasValidCSVExtension(filename string) bool {
	validExtensions := []string{".csv", ".CSV"}
	for _, ext := range validExtensions {
		if len(filename) >= len(ext) && filename[len(filename)-len(ext):] == ext {
			return true
		}
	}
	return false
}
