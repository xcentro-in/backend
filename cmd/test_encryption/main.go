package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"

	"xcentro.in/internal/shared/crypto"
)

// TestPayeeRequest matches your test body structure
type TestPayeeRequest struct {
	Payees []Payee `json:"payees"`
}

type Payee struct {
	AccountHolder string `json:"accountHolder"`
	Email         string `json:"email"`
	Mobile        string `json:"mobile"`
	AccountNumber string `json:"accountNumber"`
	IFSC          string `json:"ifsc"`
	BankName      string `json:"bankName"`
	BranchName    string `json:"branchName"`
}

func main() {
	fmt.Println("🔐 Testing Castler AES-256 Encryption/Decryption")
	fmt.Println("=" + string(make([]byte, 60)))

	// Test data matching your request
	testData := TestPayeeRequest{
		Payees: []Payee{
			{
				AccountHolder: "Shubhankar Bhadra",
				Email:         "vishalshubhankar@gmail.com",
				Mobile:        "6304126471",
				AccountNumber: "50100540083371",
				IFSC:          "HDFC0009401",
				BankName:      "HDFC",
				BranchName:    "Vittal Rao Nagar",
			},
		},
	}

	// Convert to JSON
	jsonData, err := json.MarshalIndent(testData, "", "  ")
	if err != nil {
		log.Fatalf("❌ Failed to marshal test data: %v", err)
	}

	fmt.Println("\n📄 Original JSON:")
	fmt.Println(string(jsonData))

	// Encryption configuration (same as Castler)
	encryptionKey := "your_castler_encryption_key_here" // Replace with actual key from .env
	ivBase64 := "TkMwVjAkMFQwTDAzMFJNRQ=="              // Fixed IV for Castler

	// Decode IV from base64
	fixedIV, err := base64.StdEncoding.DecodeString(ivBase64)
	if err != nil {
		log.Fatalf("❌ Failed to decode IV: %v", err)
	}

	fmt.Printf("\n🔑 Encryption Key: %s\n", encryptionKey)
	fmt.Printf("🔑 IV (Base64): %s\n", ivBase64)
	fmt.Printf("🔑 IV (Raw bytes): %v\n", fixedIV)
	fmt.Printf("🔑 IV Length: %d bytes\n", len(fixedIV))

	// Step 1: Encrypt using Castler wrapper
	fmt.Println("\n" + string(make([]byte, 60)))
	fmt.Println("🔒 ENCRYPTION STEP")
	fmt.Println(string(make([]byte, 60)))

	encryptedData, err := crypto.EncryptForCastler(string(jsonData), encryptionKey, fixedIV)
	if err != nil {
		log.Fatalf("❌ Encryption failed: %v", err)
	}

	fmt.Printf("\n✅ Encrypted Data (Base64):\n%s\n", encryptedData)
	fmt.Printf("\n📊 Encrypted Length: %d bytes\n", len(encryptedData))

	// Step 2: Decrypt using Castler wrapper
	fmt.Println("\n" + string(make([]byte, 60)))
	fmt.Println("🔓 DECRYPTION STEP")
	fmt.Println(string(make([]byte, 60)))

	decryptedData, err := crypto.DecryptFromCastler(encryptedData, encryptionKey, fixedIV)
	if err != nil {
		log.Fatalf("❌ Decryption failed: %v", err)
	}

	fmt.Printf("\n✅ Decrypted JSON:\n%s\n", decryptedData)

	// Step 3: Verify data integrity
	fmt.Println("\n" + string(make([]byte, 60)))
	fmt.Println("✓ VERIFICATION STEP")
	fmt.Println(string(make([]byte, 60)))

	var decryptedPayload TestPayeeRequest
	if err := json.Unmarshal([]byte(decryptedData), &decryptedPayload); err != nil {
		log.Fatalf("❌ Failed to parse decrypted JSON: %v", err)
	}

	// Compare original vs decrypted
	originalJSON := string(jsonData)
	if decryptedData == originalJSON {
		fmt.Println("\n✅ SUCCESS: Decrypted data matches original!")
	} else {
		// Try comparing without whitespace differences
		var original TestPayeeRequest
		json.Unmarshal(jsonData, &original)

		if fmt.Sprintf("%+v", original) == fmt.Sprintf("%+v", decryptedPayload) {
			fmt.Println("\n✅ SUCCESS: Decrypted data matches original (content-wise)!")
		} else {
			fmt.Println("\n❌ FAILED: Decrypted data does NOT match original!")
		}
	}

	// Display decrypted payee details
	fmt.Println("\n📋 Decrypted Payee Details:")
	for i, payee := range decryptedPayload.Payees {
		fmt.Printf("  Payee #%d:\n", i+1)
		fmt.Printf("    Account Holder: %s\n", payee.AccountHolder)
		fmt.Printf("    Email: %s\n", payee.Email)
		fmt.Printf("    Mobile: %s\n", payee.Mobile)
		fmt.Printf("    Account Number: %s\n", payee.AccountNumber)
		fmt.Printf("    IFSC: %s\n", payee.IFSC)
		fmt.Printf("    Bank: %s\n", payee.BankName)
		fmt.Printf("    Branch: %s\n", payee.BranchName)
	}

	fmt.Println("\n" + string(make([]byte, 60)))
	fmt.Println("🎉 Encryption/Decryption Test Completed Successfully!")
	fmt.Println(string(make([]byte, 60)))
}
