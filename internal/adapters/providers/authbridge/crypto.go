package authbridge

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"strings"
)

// encryptInput encrypts the input using AES128-CBC following AuthBridge spec
func encryptInput(inputData string, password string) (string, error) {
	// Generate random IV (16 bytes)
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("failed to generate IV: %v", err)
	}

	// Create SHA512 hash and get first 16 characters of hex digest as key
	hash := sha512.Sum512([]byte(password))
	key := []byte(fmt.Sprintf("%x", hash)[:16])

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %v", err)
	}

	// Pad the data using PKCS7
	paddedData := pkcs7Pad([]byte(inputData), aes.BlockSize)

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

// decryptResponse decrypts the API response using AES128-CBC
func decryptResponse(encryptedResponse string, password string) (string, error) {
	// Split the response to get encrypted data and IV
	parts := strings.Split(encryptedResponse, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid encrypted response format. Expected 'encrypted_data:iv'")
	}

	encryptedB64, ivB64 := parts[0], parts[1]

	// Create SHA512 hash and get first 16 characters of hex digest as key
	hash := sha512.Sum512([]byte(password))
	key := []byte(fmt.Sprintf("%x", hash)[:16])

	// Decode from base64
	encrypted, err := base64.StdEncoding.DecodeString(encryptedB64)
	if err != nil {
		return "", fmt.Errorf("failed to decode encrypted data: %v", err)
	}

	iv, err := base64.StdEncoding.DecodeString(ivB64)
	if err != nil {
		return "", fmt.Errorf("failed to decode IV: %v", err)
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %v", err)
	}

	// Decrypt using CBC mode
	mode := cipher.NewCBCDecrypter(block, iv)
	decrypted := make([]byte, len(encrypted))
	mode.CryptBlocks(decrypted, encrypted)

	// Remove padding
	unpaddedData, err := pkcs7Unpad(decrypted)
	if err != nil {
		return "", fmt.Errorf("failed to unpad data: %v", err)
	}

	return string(unpaddedData), nil
}

// pkcs7Pad adds PKCS7 padding
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := make([]byte, padding)
	for i := range padText {
		padText[i] = byte(padding)
	}
	return append(data, padText...)
}

// pkcs7Unpad removes PKCS7 padding
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