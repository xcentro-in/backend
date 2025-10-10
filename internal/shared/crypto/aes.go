package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"strings"
)

// EncryptAES256CBC encrypts the input data using AES-256-CBC with PKCS7 padding.
//
// Parameters:
//   - data: The plaintext string to encrypt
//   - password: Password used to derive the encryption key (SHA512 hash -> first 16 bytes)
//   - iv: Initialization Vector (16 bytes). If nil, a random IV is generated.
//
// Returns:
//   - Encrypted data in format "base64_encrypted_data:base64_iv"
//   - Error if encryption fails
//
// Usage:
//   - For random IV (AuthBridge, JioPay): EncryptAES256CBC(data, password, nil)
//   - For fixed IV (Easebuzz, Castler): EncryptAES256CBC(data, password, fixedIVBytes)
func EncryptAES256CBC(data string, password string, iv []byte) (string, error) {
	// Generate or validate IV
	if iv == nil {
		// Generate random IV for providers that don't specify one
		var err error
		iv, err = GenerateRandomIV()
		if err != nil {
			return "", fmt.Errorf("failed to generate IV: %w", err)
		}
	} else {
		// Validate provided IV
		if len(iv) != aes.BlockSize {
			return "", fmt.Errorf("invalid IV length: expected %d bytes, got %d bytes", aes.BlockSize, len(iv))
		}
	}

	// Derive encryption key from password
	key := DeriveKeyFromPassword(password)

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	// Pad the data using PKCS7
	paddedData := pkcs7Pad([]byte(data), aes.BlockSize)

	// Encrypt using CBC mode
	mode := cipher.NewCBCEncrypter(block, iv)
	encrypted := make([]byte, len(paddedData))
	mode.CryptBlocks(encrypted, paddedData)

	// Encode to base64
	encryptedB64 := base64.StdEncoding.EncodeToString(encrypted)
	ivB64 := base64.StdEncoding.EncodeToString(iv)

	// Return in format "encrypted_data:iv"
	return fmt.Sprintf("%s:%s", encryptedB64, ivB64), nil
}

// DecryptAES256CBC decrypts the encrypted data using AES-256-CBC.
//
// Parameters:
//   - encryptedData: Encrypted string in format "base64_encrypted_data:base64_iv"
//   - password: Password used to derive the decryption key (must match encryption password)
//
// Returns:
//   - Decrypted plaintext string
//   - Error if decryption fails
func DecryptAES256CBC(encryptedData string, password string) (string, error) {
	// Split the response to get encrypted data and IV
	parts := strings.Split(encryptedData, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid encrypted data format. Expected 'encrypted_data:iv', got %d parts", len(parts))
	}

	encryptedB64, ivB64 := parts[0], parts[1]

	// Derive decryption key from password
	key := DeriveKeyFromPassword(password)

	// Decode from base64
	encrypted, err := base64.StdEncoding.DecodeString(encryptedB64)
	if err != nil {
		return "", fmt.Errorf("failed to decode encrypted data: %w", err)
	}

	iv, err := base64.StdEncoding.DecodeString(ivB64)
	if err != nil {
		return "", fmt.Errorf("failed to decode IV: %w", err)
	}

	// Validate IV length
	if len(iv) != aes.BlockSize {
		return "", fmt.Errorf("invalid IV length: expected %d bytes, got %d bytes", aes.BlockSize, len(iv))
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	// Decrypt using CBC mode
	mode := cipher.NewCBCDecrypter(block, iv)
	decrypted := make([]byte, len(encrypted))
	mode.CryptBlocks(decrypted, encrypted)

	// Remove padding
	unpaddedData, err := pkcs7Unpad(decrypted)
	if err != nil {
		return "", fmt.Errorf("failed to unpad data: %w", err)
	}

	return string(unpaddedData), nil
}

// GenerateRandomIV generates a cryptographically secure random 16-byte IV.
func GenerateRandomIV() ([]byte, error) {
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("failed to generate random IV: %w", err)
	}
	return iv, nil
}

// DeriveKeyFromPassword derives a 16-byte encryption key from a password using SHA512.
// The key is the first 16 characters of the SHA512 hex digest.
func DeriveKeyFromPassword(password string) []byte {
	hash := sha512.Sum512([]byte(password))
	// Convert hash to hex string and take first 16 characters as key
	return []byte(fmt.Sprintf("%x", hash)[:16])
}

// pkcs7Pad adds PKCS7 padding to data to make it a multiple of blockSize.
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := make([]byte, padding)
	for i := range padText {
		padText[i] = byte(padding)
	}
	return append(data, padText...)
}

// pkcs7Unpad removes PKCS7 padding from data.
func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}

	padding := int(data[len(data)-1])
	if padding > len(data) || padding == 0 {
		return nil, fmt.Errorf("invalid padding")
	}

	// Verify padding
	for i := len(data) - padding; i < len(data); i++ {
		if data[i] != byte(padding) {
			return nil, fmt.Errorf("invalid padding")
		}
	}

	return data[:len(data)-padding], nil
}
