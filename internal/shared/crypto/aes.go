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

// EncryptionOptions provides flexible encryption configuration for different providers
type EncryptionOptions struct {
	KeySize    int    // 16 for AES-128, 32 for AES-256
	IncludeIV  bool   // true = return "encrypted:iv", false = return "encrypted" only
	IV         []byte // nil = random IV, or provide fixed IV bytes
	DerivedKey bool
}

// DecryptionOptions provides flexible decryption configuration for different providers
type DecryptionOptions struct {
	KeySize    int    // 16 for AES-128, 32 for AES-256
	IV         []byte // Required for formats without IV (like Castler)
	DerivedKey bool
}

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
// Deprecated: Use deriveKey() with explicit keySize instead
func DeriveKeyFromPassword(password string) []byte {
	return deriveKey(password, 16)
}

// deriveKey derives an encryption key of specified size from a password using SHA512
// Parameters:
//   - password: Password string to derive key from
//   - keySize: Size of key in bytes (16 for AES-128, 32 for AES-256)
//
// Returns:
//   - Key bytes of specified length
func deriveKey(password string, keySize int) []byte {
	hash := sha512.Sum512([]byte(password))
	// Convert hash to hex string and take first keySize characters
	hexStr := fmt.Sprintf("%x", hash)
	if keySize > len(hexStr) {
		keySize = len(hexStr)
	}
	return []byte(hexStr[:keySize])
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

// EncryptAES_CBC encrypts data using AES-CBC with flexible options
// This is the enhanced generic function that supports both AES-128 and AES-256
//
// Parameters:
//   - data: Plaintext string to encrypt
//   - password: Password used to derive encryption key
//   - opts: Encryption options (KeySize, IncludeIV, IV)
//
// Returns:
//   - Encrypted data as base64 string (format depends on opts.IncludeIV)
//   - Error if encryption fails
func EncryptAES_CBC(data string, password string, opts EncryptionOptions) (string, error) {
	// Validate and set defaults
	if opts.KeySize != 16 && opts.KeySize != 32 {
		return "", fmt.Errorf("invalid KeySize: must be 16 (AES-128) or 32 (AES-256)")
	}

	// Handle IV
	iv := opts.IV
	if iv == nil {
		var err error
		iv, err = GenerateRandomIV()
		if err != nil {
			return "", fmt.Errorf("failed to generate IV: %w", err)
		}
	} else if len(iv) != aes.BlockSize {
		return "", fmt.Errorf("invalid IV length: expected %d bytes, got %d bytes", aes.BlockSize, len(iv))
	}

	// Derive key based on specified size
	fmt.Printf("Password === \n %s", password)
	var key []byte
	if opts.DerivedKey {
		key = deriveKey(password, opts.KeySize)
	} else {
		key = []byte(password)
	}

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

	// Return format based on IncludeIV option
	if opts.IncludeIV {
		ivB64 := base64.StdEncoding.EncodeToString(iv)
		return fmt.Sprintf("%s:%s", encryptedB64, ivB64), nil
	}

	// Castler format: encrypted data only
	return encryptedB64, nil
}

// DecryptAES_CBC decrypts data using AES-CBC with flexible options
// This is the enhanced generic function that supports both AES-128 and AES-256
//
// Parameters:
//   - encryptedData: Base64 encoded encrypted string (may or may not include IV)
//   - password: Password used to derive decryption key
//   - opts: Decryption options (KeySize, IV if not in encryptedData)
//
// Returns:
//   - Decrypted plaintext string
//   - Error if decryption fails
func DecryptAES_CBC(encryptedData string, password string, opts DecryptionOptions) (string, error) {
	// Validate KeySize
	if opts.KeySize != 16 && opts.KeySize != 32 {
		return "", fmt.Errorf("invalid KeySize: must be 16 (AES-128) or 32 (AES-256)")
	}

	var encrypted []byte
	var iv []byte
	var err error

	// Check if IV is provided separately (Castler format)
	if opts.IV != nil {
		// IV provided in options, data is just encrypted content
		if len(opts.IV) != aes.BlockSize {
			return "", fmt.Errorf("invalid IV length: expected %d bytes, got %d bytes", aes.BlockSize, len(opts.IV))
		}
		iv = opts.IV

		// Decode encrypted data
		encrypted, err = base64.StdEncoding.DecodeString(encryptedData)
		if err != nil {
			return "", fmt.Errorf("failed to decode encrypted data: %w", err)
		}
	} else {
		// Standard format: encrypted:iv
		parts := strings.Split(encryptedData, ":")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid encrypted data format. Expected 'encrypted_data:iv', got %d parts", len(parts))
		}

		encryptedB64, ivB64 := parts[0], parts[1]

		// Decode from base64
		encrypted, err = base64.StdEncoding.DecodeString(encryptedB64)
		if err != nil {
			return "", fmt.Errorf("failed to decode encrypted data: %w", err)
		}

		iv, err = base64.StdEncoding.DecodeString(ivB64)
		if err != nil {
			return "", fmt.Errorf("failed to decode IV: %w", err)
		}

		if len(iv) != aes.BlockSize {
			return "", fmt.Errorf("invalid IV length: expected %d bytes, got %d bytes", aes.BlockSize, len(iv))
		}
	}

	// Derive key based on specified size
	var key []byte
	if opts.DerivedKey {
		key = deriveKey(password, opts.KeySize)
	} else {
		key = []byte(password)
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

// ============================================================================
// Provider-Specific Convenience Wrappers
// ============================================================================

// EncryptForCastler encrypts data for Castler API using AES-256-CBC with fixed IV
// Returns base64 encoded encrypted data (IV not included in output)
//
// Parameters:
//   - data: JSON string to encrypt
//   - encryptionKey: Secret key from config
//   - fixedIV: Fixed IV bytes (should be "NC0V0$0T0L030RME")
//
// Returns:
//   - Base64 encoded encrypted data
//   - Error if encryption fails
func EncryptForCastler(data string, encryptionKey string, fixedIV []byte) (string, error) {
	return EncryptAES_CBC(data, encryptionKey, EncryptionOptions{
		KeySize:    32,    // AES-256
		IncludeIV:  false, // Castler: don't include IV in output
		IV:         fixedIV,
		DerivedKey: false,
	})
}

// DecryptFromCastler decrypts response from Castler API using AES-256-CBC with fixed IV
// Expects base64 encoded encrypted data (without IV)
//
// Parameters:
//   - encryptedData: Base64 encoded encrypted string
//   - encryptionKey: Secret key from config
//   - fixedIV: Fixed IV bytes (should be "NC0V0$0T0L030RME")
//
// Returns:
//   - Decrypted JSON string
//   - Error if decryption fails
func DecryptFromCastler(encryptedData string, encryptionKey string, fixedIV []byte) (string, error) {
	return DecryptAES_CBC(encryptedData, encryptionKey, DecryptionOptions{
		KeySize: 32, // AES-256
		IV:      fixedIV,
	})
}

// EncryptForJioPay encrypts data for JioPay API using AES-128-CBC with random IV
// Returns format: "base64_encrypted:base64_iv"
//
// Parameters:
//   - data: JSON string to encrypt
//   - encryptionKey: Secret key from config
//
// Returns:
//   - Encrypted data in format "encrypted:iv"
//   - Error if encryption fails
func EncryptForJioPay(data string, encryptionKey string) (string, error) {
	return EncryptAES_CBC(data, encryptionKey, EncryptionOptions{
		KeySize:   16,   // AES-128
		IncludeIV: true, // JioPay: include IV in output
		IV:        nil,  // Random IV
	})
}

// DecryptFromJioPay decrypts response from JioPay API using AES-128-CBC
// Expects format: "base64_encrypted:base64_iv"
//
// Parameters:
//   - encryptedData: Encrypted string in format "encrypted:iv"
//   - encryptionKey: Secret key from config
//
// Returns:
//   - Decrypted JSON string
//   - Error if decryption fails
func DecryptFromJioPay(encryptedData string, encryptionKey string) (string, error) {
	return DecryptAES_CBC(encryptedData, encryptionKey, DecryptionOptions{
		KeySize: 16,  // AES-128
		IV:      nil, // IV is in the data
	})
}

// EncryptForEasebuzz encrypts data for Easebuzz API using AES-128-CBC with fixed IV
// Returns format: "base64_encrypted:base64_iv"
//
// Parameters:
//   - data: JSON string to encrypt
//   - encryptionKey: Secret key from config
//   - fixedIV: Fixed IV bytes from config
//
// Returns:
//   - Encrypted data in format "encrypted:iv"
//   - Error if encryption fails
func EncryptForEasebuzz(data string, encryptionKey string, fixedIV []byte) (string, error) {
	return EncryptAES_CBC(data, encryptionKey, EncryptionOptions{
		KeySize:   16,   // AES-128
		IncludeIV: true, // Easebuzz: include IV in output
		IV:        fixedIV,
	})
}

// DecryptFromEasebuzz decrypts response from Easebuzz API using AES-128-CBC
// Expects format: "base64_encrypted:base64_iv"
//
// Parameters:
//   - encryptedData: Encrypted string in format "encrypted:iv"
//   - encryptionKey: Secret key from config
//
// Returns:
//   - Decrypted JSON string
//   - Error if decryption fails
func DecryptFromEasebuzz(encryptedData string, encryptionKey string) (string, error) {
	return DecryptAES_CBC(encryptedData, encryptionKey, DecryptionOptions{
		KeySize: 16,  // AES-128
		IV:      nil, // IV is in the data
	})
}

// ============================================================================
// Bank Account Number Encryption (AES-256-GCM for PCI DSS Compliance)
// ============================================================================

// EncryptBankAccountNumber encrypts a bank account number using AES-256-GCM
// This provides authenticated encryption with additional data (AEAD) for maximum security
//
// Parameters:
//   - accountNumber: Plaintext bank account number
//   - encryptionKey: 32-byte encryption key (will be hashed if longer)
//
// Returns:
//   - Encrypted data in format "base64_encrypted:base64_nonce"
//   - Error if encryption fails
//
// Note: Uses GCM mode (not CBC) for authenticated encryption
func EncryptBankAccountNumber(accountNumber string, encryptionKey string) (string, error) {
	if accountNumber == "" {
		return "", fmt.Errorf("account number cannot be empty")
	}

	// Derive 32-byte key for AES-256
	key := deriveKey(encryptionKey, 32)

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce (12 bytes for GCM)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt and authenticate
	ciphertext := gcm.Seal(nil, nonce, []byte(accountNumber), nil)

	// Encode to base64
	encryptedB64 := base64.StdEncoding.EncodeToString(ciphertext)
	nonceB64 := base64.StdEncoding.EncodeToString(nonce)

	// Return in format "encrypted:nonce"
	return fmt.Sprintf("%s:%s", encryptedB64, nonceB64), nil
}

// DecryptBankAccountNumber decrypts a bank account number using AES-256-GCM
//
// Parameters:
//   - encryptedData: Encrypted string in format "base64_encrypted:base64_nonce"
//   - encryptionKey: 32-byte encryption key (same as used for encryption)
//
// Returns:
//   - Decrypted plaintext bank account number
//   - Error if decryption or authentication fails
func DecryptBankAccountNumber(encryptedData string, encryptionKey string) (string, error) {
	if encryptedData == "" {
		return "", fmt.Errorf("encrypted data cannot be empty")
	}

	// Split the data to get encrypted content and nonce
	parts := strings.Split(encryptedData, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid encrypted data format. Expected 'encrypted:nonce', got %d parts", len(parts))
	}

	encryptedB64, nonceB64 := parts[0], parts[1]

	// Decode from base64
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedB64)
	if err != nil {
		return "", fmt.Errorf("failed to decode encrypted data: %w", err)
	}

	nonce, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return "", fmt.Errorf("failed to decode nonce: %w", err)
	}

	// Derive 32-byte key for AES-256
	key := deriveKey(encryptionKey, 32)

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Validate nonce size
	if len(nonce) != gcm.NonceSize() {
		return "", fmt.Errorf("invalid nonce size: expected %d bytes, got %d bytes", gcm.NonceSize(), len(nonce))
	}

	// Decrypt and verify authentication
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt or authenticate: %w", err)
	}

	return string(plaintext), nil
}

// ExtractLastDigits extracts the last N digits from a bank account number
// This is a utility function to avoid exposing full account numbers
//
// Parameters:
//   - accountNumber: Full plaintext account number
//   - numDigits: Number of digits to extract (typically 4 or 5)
//
// Returns:
//   - Last N digits, or entire number if shorter than N
func ExtractLastDigits(accountNumber string, numDigits int) string {
	if len(accountNumber) <= numDigits {
		return accountNumber
	}
	return accountNumber[len(accountNumber)-numDigits:]
}
