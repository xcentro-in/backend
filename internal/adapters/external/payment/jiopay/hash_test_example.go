package jiopay

import (
	"fmt"
)

// ExampleHashGeneration demonstrates the corrected HMAC-SHA256 hash generation
// matching JioPay's Python implementation
//
// Python equivalent:
// ```python
// def append_hmac(key, json_data):
//
//	myKeys = list(json_data.keys())
//	myKeys.sort()
//	json_string = ''
//	for mykey in sorted(json_data.keys()):
//	    json_string = json_string + json_data[mykey]
//	message = json_string.encode('utf-8')
//	hmac_obj = hmac.new(key=key.encode('utf-8'), msg=message, digestmod=hashlib.sha256)
//	return hmac_obj.hexdigest()
//
// ```
func ExampleHashGeneration() {
	// Example request fields
	fieldMap := map[string]string{
		"merchantId":      "JPMID0001",
		"merchantTxnNo":   "TEST123456",
		"amount":          "100.00",
		"currencyCode":    "356",
		"payType":         "1",
		"customerEmailID": "guest@jiopay.in",
		"transactionType": "SALE",
		"returnURL":       "https://merchant.com",
		"txnDate":         "20250610125921",
		"paymentMode":     "CARD",
	}

	apiKey := "your_api_key_here"

	// Generate hash using SecureHashGenerator
	hashGen := NewSecureHashGenerator(apiKey)
	hash := hashGen.GenerateForRequest(fieldMap)

	fmt.Printf("Generated Hash: %s\n", hash)

	// How it works:
	// 1. Sort keys alphabetically:
	//    amount, currencyCode, customerEmailID, merchantId, merchantTxnNo,
	//    payType, paymentMode, returnURL, transactionType, txnDate
	//
	// 2. Concatenate VALUES in sorted key order (NO separators):
	//    "100.00356guest@jiopay.inJPMID0001TEST1234561CARDhttps://merchant.comSALE20250610125921"
	//
	// 3. HMAC-SHA256(apiKey, concatenated_string)
	//
	// 4. Return lowercase hex string
}

// VerifyHashAgainstPython allows manual verification against Python script
// Usage:
//  1. Run Python script with same inputs
//  2. Compare outputs
//  3. Should match exactly (lowercase hex, 64 characters)
func VerifyHashAgainstPython() {
	fmt.Println("=== JioPay Hash Generation Verification ===")
	fmt.Println()
	fmt.Println("Python script equivalent:")
	fmt.Println("```python")
	fmt.Println("import hmac, hashlib")
	fmt.Println()
	fmt.Println("json_data = {")
	fmt.Println("    'amount': '100.00',")
	fmt.Println("    'currencyCode': '356',")
	fmt.Println("    'customerEmailID': 'guest@jiopay.in',")
	fmt.Println("    'merchantId': 'JPMID0001',")
	fmt.Println("    'merchantTxnNo': 'TEST123456',")
	fmt.Println("    'payType': '1',")
	fmt.Println("    'paymentMode': 'CARD',")
	fmt.Println("    'returnURL': 'https://merchant.com',")
	fmt.Println("    'transactionType': 'SALE',")
	fmt.Println("    'txnDate': '20250610125921'")
	fmt.Println("}")
	fmt.Println()
	fmt.Println("key = 'your_api_key_here'")
	fmt.Println()
	fmt.Println("# Sort and concatenate")
	fmt.Println("sorted_keys = sorted(json_data.keys())")
	fmt.Println("message = ''.join([json_data[k] for k in sorted_keys])")
	fmt.Println("print('Message:', message)")
	fmt.Println()
	fmt.Println("# HMAC-SHA256")
	fmt.Println("hmac_obj = hmac.new(key.encode(), message.encode(), hashlib.sha256)")
	fmt.Println("print('Hash:', hmac_obj.hexdigest())")
	fmt.Println("```")
	fmt.Println()

	// Go implementation
	fieldMap := map[string]string{
		"amount":          "100.00",
		"currencyCode":    "356",
		"customerEmailID": "guest@jiopay.in",
		"merchantId":      "JPMID0001",
		"merchantTxnNo":   "TEST123456",
		"payType":         "1",
		"paymentMode":     "CARD",
		"returnURL":       "https://merchant.com",
		"transactionType": "SALE",
		"txnDate":         "20250610125921",
	}

	apiKey := "your_api_key_here"

	// Generate hash using SecureHashGenerator
	hashGen := NewSecureHashGenerator(apiKey)
	hash := hashGen.GenerateForRequest(fieldMap)

	fmt.Println("Go implementation result:")
	fmt.Printf("Hash: %s\n", hash)
	fmt.Println()
	fmt.Println("✓ If Python and Go hashes match, implementation is correct!")
}

// ExpectedConcatenationOrder shows the exact field order for hash generation
func ExpectedConcatenationOrder() map[string]int {
	// After alphabetical sorting, fields are concatenated in this order:
	return map[string]int{
		"accountIFSC":      1,
		"accountNo":        2,
		"addlParam1":       3,
		"addlParam2":       4,
		"aggregatorID":     5,
		"amount":           6,
		"cardExpiry":       7,
		"cardNo":           8,
		"currencyCode":     9,
		"customerEmailID":  10,
		"customerID":       11,
		"customerMobileNo": 12,
		"customerName":     13,
		"cvv":              14,
		"invoiceNo":        15,
		"merchantId":       16,
		"merchantTxnNo":    17,
		"nameOnCard":       18,
		"orderDesc":        19,
		"payType":          20,
		"paymentMode":      21,
		"returnURL":        22,
		"tokenCryptogram":  23,
		"tokenRefId":       24,
		"transactionType":  25,
		"txnDate":          26,
		// secureHash is NOT included in hash calculation
	}
}
