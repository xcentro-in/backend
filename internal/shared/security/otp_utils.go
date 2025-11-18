package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

// GenerateOTP generates a random numeric OTP of specified length (default 6 digits)
// Returns the OTP as a string (e.g., "123456")
func GenerateOTP(length int) (string, error) {
	if length <= 0 {
		length = 6 // Default to 6 digits
	}

	// Calculate max value for the given length (10^length - 1)
	max := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(length)), nil)

	// Generate random number between 0 and max
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("failed to generate OTP: %w", err)
	}

	// Format with leading zeros to ensure correct length
	format := fmt.Sprintf("%%0%dd", length)
	return fmt.Sprintf(format, n), nil
}

// HashOTP creates a SHA-256 hash of an OTP for secure database storage
// This prevents OTP compromise if the database is breached
func HashOTP(otp string) string {
	hash := sha256.Sum256([]byte(otp))
	return hex.EncodeToString(hash[:])
}

// ValidateOTP compares a plaintext OTP against a hashed OTP
// Returns true if they match, false otherwise
func ValidateOTP(plaintextOTP, hashedOTP string) bool {
	return HashOTP(plaintextOTP) == hashedOTP
}
