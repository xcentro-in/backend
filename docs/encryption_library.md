# Shared AES Encryption Library

## Overview

The XCentro backend provides a shared AES-256-CBC encryption library for securing sensitive data across multiple payment gateways and third-party integrations. The library is located at:

```
internal/shared/crypto/aes.go
```

## Design Philosophy

### Why Shared?
- **Reusability**: Single implementation serves AuthBridge, JioPay, Easebuzz, Castler
- **Consistency**: Same encryption standard across all providers
- **Maintainability**: Bug fixes and security updates in one place
- **Testability**: Centralized test coverage

### Key Features
- **AES-256-CBC encryption** with PKCS7 padding
- **Flexible IV handling**: Random generation or fixed IV support
- **Password-based key derivation** using SHA512
- **Standard format**: Returns `"base64_encrypted_data:base64_iv"`

---

## API Reference

### 1. EncryptAES256CBC

```go
func EncryptAES256CBC(data string, password string, iv []byte) (string, error)
```

**Purpose**: Encrypts plaintext data using AES-256-CBC

**Parameters**:
- `data` - Plaintext string to encrypt
- `password` - Password for key derivation (converted to 16-byte key via SHA512)
- `iv` - Initialization Vector (16 bytes)
  - If `nil`: Generates random IV (use for AuthBridge, JioPay)
  - If provided: Uses fixed IV (use for Easebuzz, Castler)

**Returns**:
- Encrypted string in format: `"base64_encrypted_data:base64_iv"`
- Error if encryption fails

**Example (Random IV)**:
```go
import "xcentro.in/internal/shared/crypto"

// AuthBridge / JioPay - Random IV generation
encrypted, err := crypto.EncryptAES256CBC(jsonData, password, nil)
if err != nil {
    return fmt.Errorf("encryption failed: %w", err)
}
// encrypted = "sG4xK9...H8pQ==:7fJ2...kL0="
```

**Example (Fixed IV)**:
```go
// Easebuzz / Castler - Fixed IV from config
ivBytes, _ := base64.StdEncoding.DecodeString(cfg.Easebuzz.EncryptionIV)
encrypted, err := crypto.EncryptAES256CBC(jsonData, cfg.Easebuzz.EncryptionKey, ivBytes)
if err != nil {
    return fmt.Errorf("encryption failed: %w", err)
}
```

---

### 2. DecryptAES256CBC

```go
func DecryptAES256CBC(encryptedData string, password string) (string, error)
```

**Purpose**: Decrypts AES-256-CBC encrypted data

**Parameters**:
- `encryptedData` - Encrypted string in format `"base64_encrypted_data:base64_iv"`
- `password` - Same password used for encryption

**Returns**:
- Decrypted plaintext string
- Error if decryption fails

**Example**:
```go
decrypted, err := crypto.DecryptAES256CBC(encryptedResponse, password)
if err != nil {
    return fmt.Errorf("decryption failed: %w", err)
}
```

---

### 3. GenerateRandomIV

```go
func GenerateRandomIV() ([]byte, error)
```

**Purpose**: Generates a cryptographically secure random 16-byte IV

**Returns**:
- 16-byte IV slice
- Error if random generation fails

**Example**:
```go
iv, err := crypto.GenerateRandomIV()
if err != nil {
    return fmt.Errorf("IV generation failed: %w", err)
}
```

---

### 4. DeriveKeyFromPassword

```go
func DeriveKeyFromPassword(password string) []byte
```

**Purpose**: Derives a 16-byte encryption key from password using SHA512

**Parameters**:
- `password` - Input password

**Returns**:
- 16-byte key (first 16 characters of SHA512 hex digest)

**Example**:
```go
key := crypto.DeriveKeyFromPassword("mySecretPassword")
// key = []byte of length 16
```

---

## Provider-Specific Usage

### AuthBridge (Document Verification)

**IV Strategy**: Random (generate new IV per request)

**Location**: `internal/adapters/providers/authbridge/client.go`

```go
import "xcentro.in/internal/shared/crypto"

// Encrypt input data
encryptedInput, err := crypto.EncryptAES256CBC(inputJSON, c.config.Password, nil)
if err != nil {
    return nil, fmt.Errorf("encryption failed: %v", err)
}

// Decrypt API response
decryptedResponse, err := crypto.DecryptAES256CBC(apiResp.ResponseData, c.config.Password)
if err != nil {
    return nil, fmt.Errorf("decryption failed: %v", err)
}
```

**Environment Variables**:
```bash
TRUTHSCREEN_USERNAME=your_username
TRUTHSCREEN_PASSWORD=your_password  # Used for key derivation
```

---

### JioPay (Payment Gateway)

**IV Strategy**: Random (generate new IV per transaction)

**Future Implementation**: `internal/adapters/external/payment/jiopay/client.go`

```go
import "xcentro.in/internal/shared/crypto"

// Encrypt payment request
encryptedPayload, err := crypto.EncryptAES256CBC(paymentJSON, cfg.JioPay.EncryptionKey, nil)
if err != nil {
    return nil, fmt.Errorf("encryption failed: %w", err)
}
```

**Environment Variables**:
```bash
JIOPAY_BASE_URL=https://api.jiopay.com
JIOPAY_MERCHANT_ID=your_merchant_id
JIOPAY_API_KEY=your_api_key
JIOPAY_ENCRYPTION_KEY=your_encryption_key  # Optional
JIOPAY_WEBHOOK_SECRET=your_webhook_secret
```

---

### Easebuzz (Payment Gateway)

**IV Strategy**: Fixed (use provider-specified IV)

**Future Implementation**: `internal/adapters/external/payment/easebuzz/client.go`

```go
import (
    "encoding/base64"
    "xcentro.in/internal/shared/crypto"
)

// Decode fixed IV from config
ivBytes, err := base64.StdEncoding.DecodeString(cfg.Easebuzz.EncryptionIV)
if err != nil {
    return nil, fmt.Errorf("invalid IV in config: %w", err)
}

// Encrypt payment request with fixed IV
encryptedPayload, err := crypto.EncryptAES256CBC(
    paymentJSON,
    cfg.Easebuzz.EncryptionKey,
    ivBytes,  // Fixed IV from environment
)
if err != nil {
    return nil, fmt.Errorf("encryption failed: %w", err)
}
```

**Environment Variables**:
```bash
EASEBUZZ_BASE_URL=https://api.easebuzz.in
EASEBUZZ_MERCHANT_KEY=your_merchant_key
EASEBUZZ_SALT=your_salt
EASEBUZZ_ENCRYPTION_KEY=your_encryption_key
EASEBUZZ_ENCRYPTION_IV=base64_encoded_iv  # Fixed IV (16 bytes base64)
EASEBUZZ_WEBHOOK_SECRET=your_webhook_secret
```

**Note**: Contact Easebuzz support for the fixed IV value.

---

### Castler (Payout Provider)

**IV Strategy**: Fixed (use provider-specified IV) - **Optional**

**Location**: `internal/adapters/external/payout/castler/client.go`

```go
import (
    "encoding/base64"
    "xcentro.in/internal/shared/crypto"
)

// Only if Castler requires encryption
if cfg.Castler.EncryptionKey != "" {
    ivBytes, err := base64.StdEncoding.DecodeString(cfg.Castler.EncryptionIV)
    if err != nil {
        return nil, fmt.Errorf("invalid IV in config: %w", err)
    }

    encryptedPayload, err := crypto.EncryptAES256CBC(
        payoutJSON,
        cfg.Castler.EncryptionKey,
        ivBytes,
    )
    if err != nil {
        return nil, fmt.Errorf("encryption failed: %w", err)
    }
}
```

**Environment Variables**:
```bash
CASTLER_BASE_URL=https://api.castler.in
CASTLER_API_KEY=your_api_key
CASTLER_SECRET_KEY=your_secret_key
CASTLER_WEBHOOK_SECRET=your_webhook_secret
CASTLER_ENCRYPTION_KEY=your_encryption_key       # Optional
CASTLER_ENCRYPTION_IV=base64_encoded_iv          # Optional fixed IV
```

---

## Configuration

### Config Structs

All encryption settings are defined in `internal/shared/config/config.go`:

```go
type JioPayConfig struct {
    BaseURL       string `env:"JIOPAY_BASE_URL"`
    MerchantID    string `env:"JIOPAY_MERCHANT_ID"`
    APIKey        string `env:"JIOPAY_API_KEY"`
    EncryptionKey string `env:"JIOPAY_ENCRYPTION_KEY"`  // Optional
    WebhookSecret string `env:"JIOPAY_WEBHOOK_SECRET"`
}

type EasebuzzConfig struct {
    BaseURL       string `env:"EASEBUZZ_BASE_URL"`
    MerchantKey   string `env:"EASEBUZZ_MERCHANT_KEY"`
    Salt          string `env:"EASEBUZZ_SALT"`
    EncryptionKey string `env:"EASEBUZZ_ENCRYPTION_KEY"`
    EncryptionIV  string `env:"EASEBUZZ_ENCRYPTION_IV"`  // Fixed IV (base64)
    WebhookSecret string `env:"EASEBUZZ_WEBHOOK_SECRET"`
}

type CastlerConfig struct {
    BaseURL       string `env:"CASTLER_BASE_URL"`
    APIKey        string `env:"CASTLER_API_KEY"`
    SecretKey     string `env:"CASTLER_SECRET_KEY"`
    WebhookSecret string `env:"CASTLER_WEBHOOK_SECRET"`
    EncryptionKey string `env:"CASTLER_ENCRYPTION_KEY"`  // Optional
    EncryptionIV  string `env:"CASTLER_ENCRYPTION_IV"`   // Optional fixed IV
}
```

### Secret Redaction

All sensitive fields are automatically redacted in logs via the `Safe()` method:

```go
cfg := config.MustLoad("")
safeCfg := cfg.Safe()  // All encryption keys/IVs redacted to "***REDACTED***"
```

---

## Security Best Practices

### 1. IV Handling
- **Random IV**: Preferred for security (each encryption produces different ciphertext)
- **Fixed IV**: Only use when provider requires it (reduces security, but necessary for API compatibility)
- Never hardcode IVs in code - always use environment variables

### 2. Key Management
- Store encryption keys in environment variables, never in code
- Use different keys per environment (dev, staging, production)
- Rotate keys periodically (coordinate with provider support)
- Use AWS Secrets Manager or similar for production

### 3. Password Derivation
- Current implementation: SHA512 hash → first 16 bytes
- Not suitable for user passwords (no salt, no iterations)
- Only use for API credential derivation as specified by providers

### 4. Error Handling
- Never log plaintext data or encryption keys
- Log only generic error messages (e.g., "encryption failed")
- Use structured logging with redacted fields

### 5. Testing
- Use test fixtures with known plaintext/ciphertext pairs
- Test IV validation (16-byte length)
- Test both random and fixed IV paths
- Mock encryption in unit tests for non-crypto logic

---

## Format Specification

### Output Format
```
base64_encrypted_data:base64_iv
```

**Example**:
```
sG4xK9mL3pQ1vN8fH2jK5wT7yU9oI0pA3sD6gH8jK1lM2nB4vC7xZ9aS3dF5gH7jK9lM:7fJ2kL9mN1pQ3rS5tU7vW9xY1zA3bC5dE7fG9hI1jK3lM5nO7pQ9rS1tU3vW5xY7zA9
```

**Parsing**:
```go
parts := strings.Split(encrypted, ":")
encryptedDataBase64 := parts[0]
ivBase64 := parts[1]
```

---

## Migration Guide (AuthBridge)

### Before (Old Implementation)
```go
// internal/adapters/providers/authbridge/crypto.go
encryptedInput, err := encryptInput(inputJSON, c.config.Password)
decryptedResponse, err := decryptResponse(apiResp.ResponseData, c.config.Password)
```

### After (Shared Library)
```go
// internal/adapters/providers/authbridge/client.go
import "xcentro.in/internal/shared/crypto"

encryptedInput, err := crypto.EncryptAES256CBC(inputJSON, c.config.Password, nil)
decryptedResponse, err := crypto.DecryptAES256CBC(apiResp.ResponseData, c.config.Password)
```

**Changes**:
- ✅ Deleted `internal/adapters/providers/authbridge/crypto.go`
- ✅ Added import `"xcentro.in/internal/shared/crypto"`
- ✅ Updated function calls to use exported names
- ✅ Added `nil` parameter for random IV generation

---

## Testing

### Unit Test Example

```go
package crypto_test

import (
    "encoding/base64"
    "testing"

    "xcentro.in/internal/shared/crypto"
)

func TestEncryptDecrypt_RandomIV(t *testing.T) {
    plaintext := "Hello, World!"
    password := "mySecretPassword"

    // Encrypt with random IV
    encrypted, err := crypto.EncryptAES256CBC(plaintext, password, nil)
    if err != nil {
        t.Fatalf("Encryption failed: %v", err)
    }

    // Decrypt
    decrypted, err := crypto.DecryptAES256CBC(encrypted, password)
    if err != nil {
        t.Fatalf("Decryption failed: %v", err)
    }

    if decrypted != plaintext {
        t.Errorf("Expected %q, got %q", plaintext, decrypted)
    }
}

func TestEncryptDecrypt_FixedIV(t *testing.T) {
    plaintext := "Test payload"
    password := "fixedIVPassword"
    fixedIV := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

    // Encrypt with fixed IV
    encrypted, err := crypto.EncryptAES256CBC(plaintext, password, fixedIV)
    if err != nil {
        t.Fatalf("Encryption failed: %v", err)
    }

    // Decrypt
    decrypted, err := crypto.DecryptAES256CBC(encrypted, password)
    if err != nil {
        t.Fatalf("Decryption failed: %v", err)
    }

    if decrypted != plaintext {
        t.Errorf("Expected %q, got %q", plaintext, decrypted)
    }
}

func TestInvalidIVLength(t *testing.T) {
    invalidIV := []byte{1, 2, 3} // Too short
    _, err := crypto.EncryptAES256CBC("test", "password", invalidIV)
    if err == nil {
        t.Error("Expected error for invalid IV length")
    }
}
```

---

## Common Issues

### Issue 1: "invalid IV length" Error
**Cause**: Provided IV is not exactly 16 bytes
**Solution**:
```go
// Correct: Generate 16-byte IV
iv, err := crypto.GenerateRandomIV()

// Correct: Decode base64 IV from config (must be 16 bytes)
ivBytes, err := base64.StdEncoding.DecodeString(cfg.Easebuzz.EncryptionIV)
```

### Issue 2: Decryption Fails After Encryption
**Cause**: Different passwords used for encryption and decryption
**Solution**: Ensure same password is used:
```go
encrypted, _ := crypto.EncryptAES256CBC(data, password, nil)
decrypted, _ := crypto.DecryptAES256CBC(encrypted, password)  // Same password
```

### Issue 3: "invalid encrypted data format" Error
**Cause**: Input string doesn't match `"encrypted:iv"` format
**Solution**:
- Ensure encryption output is not truncated
- Don't split or modify the encrypted string
- Check API response format matches expected structure

---

## Future Enhancements

1. **Support for AES-GCM**: For authenticated encryption (more secure than CBC)
2. **Key rotation**: Automatic key versioning and rotation
3. **Hardware security module (HSM)**: Integration for key storage
4. **PBKDF2 key derivation**: Replace SHA512 with proper KDF (salted, iterated)
5. **Encryption context**: Add metadata for audit trails

---

## References

- [NIST AES Specification](https://csrc.nist.gov/publications/detail/fips/197/final)
- [RFC 2315 - PKCS #7](https://www.rfc-editor.org/rfc/rfc2315)
- [Go crypto/aes Documentation](https://pkg.go.dev/crypto/aes)
- [Go crypto/cipher Documentation](https://pkg.go.dev/crypto/cipher)

---

## Support

For questions or issues with the encryption library:
1. Check this documentation first
2. Review test cases in `internal/shared/crypto/aes_test.go` (if available)
3. Contact the backend team lead
4. For provider-specific issues, consult provider documentation:
   - AuthBridge: https://docs.authbridge.com
   - JioPay: https://docs.jiopay.com
   - Easebuzz: https://docs.easebuzz.in
   - Castler: https://docs.castler.in
