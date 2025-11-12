package utils

import (
	"bytes"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"
)

func TestHasValidCSVExtension(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		expected bool
	}{
		{
			name:     "Valid lowercase .csv",
			filename: "test.csv",
			expected: true,
		},
		{
			name:     "Valid uppercase .CSV",
			filename: "test.CSV",
			expected: true,
		},
		{
			name:     "Invalid .txt extension",
			filename: "test.txt",
			expected: false,
		},
		{
			name:     "Invalid .xlsx extension",
			filename: "test.xlsx",
			expected: false,
		},
		{
			name:     "No extension",
			filename: "test",
			expected: false,
		},
		{
			name:     "Multiple dots",
			filename: "test.data.csv",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasValidCSVExtension(tt.filename)
			if result != tt.expected {
				t.Errorf("hasValidCSVExtension(%q) = %v, want %v", tt.filename, result, tt.expected)
			}
		})
	}
}

func TestIsBinaryExecutable(t *testing.T) {
	tests := []struct {
		name     string
		mimeType string
		expected bool
	}{
		{
			name:     "Text CSV - safe",
			mimeType: "text/csv",
			expected: false,
		},
		{
			name:     "Executable",
			mimeType: "application/x-executable",
			expected: true,
		},
		{
			name:     "Shell script",
			mimeType: "application/x-shellscript",
			expected: true,
		},
		{
			name:     "Python script",
			mimeType: "application/x-python",
			expected: true,
		},
		{
			name:     "PHP script",
			mimeType: "application/x-php",
			expected: true,
		},
		{
			name:     "JavaScript",
			mimeType: "application/javascript",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBinaryExecutable(tt.mimeType)
			if result != tt.expected {
				t.Errorf("isBinaryExecutable(%q) = %v, want %v", tt.mimeType, result, tt.expected)
			}
		})
	}
}

func TestIsArchive(t *testing.T) {
	tests := []struct {
		name     string
		mimeType string
		expected bool
	}{
		{
			name:     "Text CSV - safe",
			mimeType: "text/csv",
			expected: false,
		},
		{
			name:     "ZIP archive",
			mimeType: "application/zip",
			expected: true,
		},
		{
			name:     "RAR archive",
			mimeType: "application/x-rar-compressed",
			expected: true,
		},
		{
			name:     "GZIP archive",
			mimeType: "application/gzip",
			expected: true,
		},
		{
			name:     "TAR archive",
			mimeType: "application/x-tar",
			expected: true,
		},
		{
			name:     "7Z archive",
			mimeType: "application/x-7z-compressed",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isArchive(tt.mimeType)
			if result != tt.expected {
				t.Errorf("isArchive(%q) = %v, want %v", tt.mimeType, result, tt.expected)
			}
		})
	}
}

func TestIsTextBasedMIME(t *testing.T) {
	tests := []struct {
		name     string
		mimeType string
		expected bool
	}{
		{
			name:     "text/csv",
			mimeType: "text/csv",
			expected: true,
		},
		{
			name:     "text/plain",
			mimeType: "text/plain",
			expected: true,
		},
		{
			name:     "text/html",
			mimeType: "text/html",
			expected: true,
		},
		{
			name:     "application/csv",
			mimeType: "application/csv",
			expected: true,
		},
		{
			name:     "application/json",
			mimeType: "application/json",
			expected: false,
		},
		{
			name:     "application/zip",
			mimeType: "application/zip",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTextBasedMIME(tt.mimeType)
			if result != tt.expected {
				t.Errorf("isTextBasedMIME(%q) = %v, want %v", tt.mimeType, result, tt.expected)
			}
		})
	}
}

// createMockFileHeader creates a mock multipart.FileHeader for testing
func createMockFileHeader(filename string, content []byte) *multipart.FileHeader {
	// Create a buffer to write the multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Create form file
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	h.Set("Content-Type", "text/csv")

	part, _ := writer.CreatePart(h)
	part.Write(content)
	writer.Close()

	// Parse the multipart form
	reader := multipart.NewReader(body, writer.Boundary())
	form, _ := reader.ReadForm(int64(len(content) + 1024))

	if len(form.File["file"]) > 0 {
		return form.File["file"][0]
	}

	return nil
}

func TestValidateUploadedFile_FileSize(t *testing.T) {
	tests := []struct {
		name      string
		filename  string
		content   []byte
		wantError bool
		errorCode string
	}{
		{
			name:      "Valid file size",
			filename:  "test.csv",
			content:   []byte("payee_name,amount\nTest,1000\n"),
			wantError: false,
		},
		{
			name:      "File too small",
			filename:  "test.csv",
			content:   []byte("test"),
			wantError: true,
			errorCode: "FILE_TOO_SMALL",
		},
		{
			name:      "Large valid CSV file",
			filename:  "test.csv",
			content:   []byte("payee_name,amount\n" + strings.Repeat("Test Payee,1000.00\n", 10000)),
			wantError: false,
		},
		{
			name:      "File size exceeds limit",
			filename:  "test.csv",
			content:   make([]byte, MaxFileSize+1),
			wantError: true,
			errorCode: "FILE_TOO_LARGE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileHeader := createMockFileHeader(tt.filename, tt.content)
			if fileHeader == nil {
				t.Skip("Could not create mock file header")
				return
			}

			err := ValidateUploadedFile(fileHeader)
			if tt.wantError {
				if err == nil {
					t.Errorf("ValidateUploadedFile() expected error but got none")
				} else if tt.errorCode != "" {
					if validationErr, ok := err.(*FileValidationError); ok {
						if validationErr.Code != tt.errorCode {
							t.Errorf("ValidateUploadedFile() error code = %q, want %q", validationErr.Code, tt.errorCode)
						}
					}
				}
			} else {
				if err != nil {
					t.Errorf("ValidateUploadedFile() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidateUploadedFile_Extension(t *testing.T) {
	tests := []struct {
		name      string
		filename  string
		wantError bool
	}{
		{
			name:      "Valid .csv extension",
			filename:  "test.csv",
			wantError: false,
		},
		{
			name:      "Valid .CSV extension",
			filename:  "test.CSV",
			wantError: false,
		},
		{
			name:      "Invalid .txt extension",
			filename:  "test.txt",
			wantError: true,
		},
		{
			name:      "Invalid .exe extension",
			filename:  "malware.exe",
			wantError: true,
		},
		{
			name:      "Invalid .xlsx extension",
			filename:  "test.xlsx",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create valid CSV content
			content := []byte("payee_name,amount\nTest Payee,1000.00\n")
			fileHeader := createMockFileHeader(tt.filename, content)
			if fileHeader == nil {
				t.Skip("Could not create mock file header")
				return
			}

			err := ValidateUploadedFile(fileHeader)
			if tt.wantError {
				if err == nil {
					t.Errorf("ValidateUploadedFile() expected error for %q but got none", tt.filename)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateUploadedFile() unexpected error for %q = %v", tt.filename, err)
				}
			}
		})
	}
}
