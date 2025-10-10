package easebuzz

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// EncryptCardField encrypts a single card field using Easebuzz-specific encryption.
//
// Easebuzz Requirements:
//   - Algorithm: AES-256-CBC
//   - Key: SHA-256 hash of EASEBUZZ_MERCHANT_KEY → first 32 bytes
//   - IV: SHA-256 hash of EASEBUZZ_SALT → first 16 bytes (FIXED)
//   - Padding: PKCS7 (multiples of 16)
//   - Output: Base64 encoded ciphertext ONLY (no IV appended)
//
// This is different from the shared crypto library which uses:
//   - AES-128 (16-byte key from SHA-512)
//   - Random or fixed IV with format "encrypted:iv"
//
// Parameters:
//   - plaintext: The field value to encrypt (card_token, cryptogram, token_expiry_date, card_cvv)
//   - merchantKey: EASEBUZZ_MERCHANT_KEY from environment
//   - salt: EASEBUZZ_SALT from environment
//
// Returns:
//   - Base64 encoded ciphertext (NO IV appended, IV is derived from salt)
//
// Usage:
//
//	encryptedToken, err := EncryptCardField(token, merchantKey, salt)
//	// encryptedToken = "sG4xK9mL3pQ1vN8fH2jK5wT7yU9oI0pA=="
func EncryptCardField(plaintext, merchantKey, salt string) (string, error) {
	if plaintext == "" {
		return "", fmt.Errorf("plaintext cannot be empty")
	}
	if merchantKey == "" {
		return "", fmt.Errorf("merchantKey cannot be empty")
	}
	if salt == "" {
		return "", fmt.Errorf("salt cannot be empty")
	}

	// Derive 32-byte AES-256 key from merchantKey
	key := deriveAES256Key(merchantKey)

	// Derive 16-byte fixed IV from salt
	iv := deriveFixedIV(salt)

	// Create AES cipher with 32-byte key
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Pad the plaintext using PKCS7
	paddedData := pkcs7Pad([]byte(plaintext), aes.BlockSize)

	// Encrypt using CBC mode
	mode := cipher.NewCBCEncrypter(block, iv)
	encrypted := make([]byte, len(paddedData))
	mode.CryptBlocks(encrypted, paddedData)

	// Base64 encode (NO IV appended - IV is fixed and derived from salt)
	encryptedB64 := base64.StdEncoding.EncodeToString(encrypted)

	return encryptedB64, nil
}

// deriveAES256Key derives a 32-byte AES-256 key from merchantKey using SHA-256.
//
// Easebuzz Spec: SHA-256(merchantKey) → first 32 bytes
func deriveAES256Key(merchantKey string) []byte {
	hash := sha256.Sum256([]byte(merchantKey))
	return hash[:32] // Return all 32 bytes for AES-256
}

// deriveFixedIV derives a 16-byte fixed IV from salt using SHA-256.
//
// Easebuzz Spec: SHA-256(salt) → first 16 bytes
func deriveFixedIV(salt string) []byte {
	hash := sha256.Sum256([]byte(salt))
	return hash[:16] // Return first 16 bytes for IV
}

// pkcs7Pad adds PKCS7 padding to data to make it a multiple of blockSize.
//
// PKCS7 padding scheme:
//   - If data length is N and block size is B
//   - Padding needed: P = B - (N % B)
//   - Append P bytes, each with value P
//
// Example: For block size 16 and data "Hello" (5 bytes):
//   - Padding needed: 16 - (5 % 16) = 11
//   - Padded data: "Hello\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b"
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := make([]byte, padding)
	for i := range padText {
		padText[i] = byte(padding)
	}
	return append(data, padText...)
}

// pkcs7Unpad removes PKCS7 padding from data.
//
// Used for decryption (if needed in webhook processing)
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

// EncryptFullPayload encrypts the entire JSON payload for tokenization API.
//
// This is different from field-level encryption used in payment APIs.
// For tokenization, the entire pre-request script parameters are encrypted.
//
// Parameters:
//   - jsonPayload: JSON string of the TokenizationRequest
//   - sKey: Encryption key (s_key) - derived from merchantKey
//   - sIV: Initialization vector (s_iv) - derived from salt
//
// Returns:
//   - Base64 encoded encrypted payload to be sent as "enc_data"
//
// Usage:
//
//	jsonData, _ := json.Marshal(tokenizationReq)
//	encData, _ := EncryptFullPayload(string(jsonData), sKey, sIV)
func EncryptFullPayload(jsonPayload string, sKey []byte, sIV []byte) (string, error) {
	if jsonPayload == "" {
		return "", fmt.Errorf("jsonPayload cannot be empty")
	}
	if len(sKey) != 32 {
		return "", fmt.Errorf("s_key must be 32 bytes for AES-256, got %d bytes", len(sKey))
	}
	if len(sIV) != 16 {
		return "", fmt.Errorf("s_iv must be 16 bytes, got %d bytes", len(sIV))
	}

	// Create AES cipher with 32-byte key
	block, err := aes.NewCipher(sKey)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Pad the payload using PKCS7
	paddedData := pkcs7Pad([]byte(jsonPayload), aes.BlockSize)

	// Encrypt using CBC mode
	mode := cipher.NewCBCEncrypter(block, sIV)
	encrypted := make([]byte, len(paddedData))
	mode.CryptBlocks(encrypted, paddedData)

	// Base64 encode the encrypted data
	encryptedB64 := base64.StdEncoding.EncodeToString(encrypted)

	return encryptedB64, nil
}

// DecryptFullPayload decrypts the encrypted payload from tokenization API response.
//
// Used for decrypting API responses if needed.
func DecryptFullPayload(encryptedB64 string, sKey []byte, sIV []byte) (string, error) {
	if encryptedB64 == "" {
		return "", fmt.Errorf("encryptedData cannot be empty")
	}
	if len(sKey) != 32 {
		return "", fmt.Errorf("s_key must be 32 bytes for AES-256")
	}
	if len(sIV) != 16 {
		return "", fmt.Errorf("s_iv must be 16 bytes")
	}

	// Decode from base64
	encrypted, err := base64.StdEncoding.DecodeString(encryptedB64)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	// Create AES cipher
	block, err := aes.NewCipher(sKey)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Decrypt using CBC mode
	mode := cipher.NewCBCDecrypter(block, sIV)
	decrypted := make([]byte, len(encrypted))
	mode.CryptBlocks(decrypted, encrypted)

	// Remove padding
	unpaddedData, err := pkcs7Unpad(decrypted)
	if err != nil {
		return "", fmt.Errorf("failed to unpad data: %w", err)
	}

	return string(unpaddedData), nil
}
