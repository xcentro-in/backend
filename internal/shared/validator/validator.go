package validator

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/shared/security"
)

// CustomValidator wraps the validator instance for Echo
type CustomValidator struct {
	validator *validator.Validate
}

// NewCustomValidator creates a new custom validator instance with custom validation rules
func NewCustomValidator() *CustomValidator {
	v := validator.New()

	// Register custom validators
	v.RegisterValidation("pan", validatePAN)
	v.RegisterValidation("gstin", validateGSTIN)
	v.RegisterValidation("aadhaar", validateAadhaar)
	v.RegisterValidation("cin", validateCIN)
	v.RegisterValidation("phone_in", validateIndianPhone)
	v.RegisterValidation("strong_password", validateStrongPassword)
	v.RegisterValidation("ifsc", validateIFSC)

	// Register character type validators
	v.RegisterValidation("business_name", validateBusinessName)
	v.RegisterValidation("person_name", validatePersonName)
	v.RegisterValidation("safe_text", validateSafeText)
	v.RegisterValidation("oauth_code", validateOAuthCode)

	return &CustomValidator{validator: v}
}

// Validate implements echo.Validator interface
// Automatically trims all string fields before validation
func (cv *CustomValidator) Validate(i interface{}) error {
	// Trim all string fields first
	TrimStrings(i)

	// Then validate
	if err := cv.validator.Struct(i); err != nil {
		// Type assert to validator.ValidationErrors
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			// Format the errors into a user-friendly map
			return &ValidationError{
				Errors: formatValidationErrors(validationErrors),
			}
		}
		return err
	}
	return nil
}

// Var validates a single variable against validation tags
// This allows handlers to validate individual values like headers, query params, etc.
func (cv *CustomValidator) Var(field interface{}, tag string) error {
	return cv.validator.Var(field, tag)
}

// ValidationError represents validation errors
type ValidationError struct {
	Errors map[string]string
}

func (ve *ValidationError) Error() string {
	return "Validation failed"
}

// formatValidationErrors converts validator errors to a map of field:message
func formatValidationErrors(errs validator.ValidationErrors) map[string]string {
	errorMap := make(map[string]string)
	for _, err := range errs {
		field := strings.ToLower(err.Field()[:1]) + err.Field()[1:] // Convert to camelCase
		errorMap[field] = getErrorMessage(err)
	}
	return errorMap
}

// getErrorMessage returns a user-friendly error message for validation errors
func getErrorMessage(err validator.FieldError) string {
	field := err.Field()
	tag := err.Tag()

	switch tag {
	case "required":
		return fmt.Sprintf("%s is required", field)
	case "email":
		return fmt.Sprintf("%s must be a valid email address", field)
	case "min":
		return fmt.Sprintf("%s must be at least %s characters long", field, err.Param())
	case "max":
		return fmt.Sprintf("%s must not exceed %s characters", field, err.Param())
	case "pan":
		return fmt.Sprintf("%s must be a valid PAN number (format: AAAAA9999A)", field)
	case "gstin":
		return fmt.Sprintf("%s must be a valid GSTIN number (format: 22AAAAA9999A1Z5)", field)
	case "aadhaar":
		return fmt.Sprintf("%s must be a valid 12-digit Aadhaar number", field)
	case "cin":
		return fmt.Sprintf("%s must be a valid CIN number (format: L12345KA2010PLC012345)", field)
	case "phone_in":
		return fmt.Sprintf("%s must be a valid 10-digit Indian phone number", field)
	case "strong_password":
		return "password must be at least 8 characters long and contain at least one uppercase letter, one lowercase letter, one number, and one special character"
	case "oneof":
		return fmt.Sprintf("%s must be one of: %s", field, err.Param())
	case "gte":
		return fmt.Sprintf("%s must be greater than or equal to %s", field, err.Param())
	case "lte":
		return fmt.Sprintf("%s must be less than or equal to %s", field, err.Param())
	case "gt":
		return fmt.Sprintf("%s must be greater than %s", field, err.Param())
	case "lt":
		return fmt.Sprintf("%s must be less than %s", field, err.Param())
	case "ifsc":
		return fmt.Sprintf("%s must be a valid IFSC code (format: ABCD0123456)", field)
	case "business_name":
		return fmt.Sprintf("%s can only contain letters, numbers, spaces, and .,&()'-/@#", field)
	case "person_name":
		return fmt.Sprintf("%s can only contain letters, spaces, and .-'", field)
	case "safe_text":
		return fmt.Sprintf("%s contains invalid characters", field)
	case "oauth_code":
		return fmt.Sprintf("%s must be a valid OAuth authorization code", field)
	case "uuid":
		return fmt.Sprintf("%s must be a valid UUID", field)
	default:
		return fmt.Sprintf("%s is invalid", field)
	}
}

// validatePAN validates Indian PAN format: AAAAA9999A
// Note: Input is already trimmed by TrimStrings() before validation
func validatePAN(fl validator.FieldLevel) bool {
	pan := fl.Field().String()
	panRegex := regexp.MustCompile(`^[A-Z]{5}[0-9]{4}[A-Z]{1}$`)
	return panRegex.MatchString(pan)
}

// validateGSTIN validates Indian GSTIN format: 22AAAAA9999A1Z5
// Note: Input is already trimmed by TrimStrings() before validation
func validateGSTIN(fl validator.FieldLevel) bool {
	gstin := fl.Field().String()
	gstinRegex := regexp.MustCompile(`^[0-9]{2}[A-Z]{5}[0-9]{4}[A-Z]{1}[1-9A-Z]{1}Z[0-9A-Z]{1}$`)
	return gstinRegex.MatchString(gstin)
}

// validateAadhaar validates Indian Aadhaar format: 2345 6789 0123 (with optional spaces)
// Must start with 2-9 (not 0 or 1), exactly 12 digits
// Note: Input is already trimmed by TrimStrings() before validation
func validateAadhaar(fl validator.FieldLevel) bool {
	aadhaar := fl.Field().String()
	// Allow optional spaces: 2345 6789 0123 or 234567890123
	aadhaarRegex := regexp.MustCompile(`^[2-9]{1}[0-9]{3}\s?[0-9]{4}\s?[0-9]{4}$`)
	return aadhaarRegex.MatchString(aadhaar)
}

// validateCIN validates Indian Company Identification Number format: L12345KA2010PLC012345
// Starts with L or U, 21 characters total
// Note: Input is already trimmed by TrimStrings() before validation
func validateCIN(fl validator.FieldLevel) bool {
	cin := fl.Field().String()
	cinRegex := regexp.MustCompile(`^[LU][0-9]{5}[A-Z]{2}[0-9]{4}[A-Z]{3}[0-9]{6}$`)
	return cinRegex.MatchString(cin)
}

// validateIndianPhone validates 10-digit Indian phone number
// Note: Input is already trimmed by TrimStrings() before validation
func validateIndianPhone(fl validator.FieldLevel) bool {
	phone := fl.Field().String()
	// Must be exactly 10 digits
	phoneRegex := regexp.MustCompile(`^[0-9]{10}$`)
	return phoneRegex.MatchString(phone)
}

// validateStrongPassword validates password strength using existing password utils
func validateStrongPassword(fl validator.FieldLevel) bool {
	password := fl.Field().String()
	return security.ValidatePasswordStrength(password) == nil
}

// validateIFSC validates Indian IFSC code format: ABCD0123456 (4 letters, 7 digits with 5th char as 0)
// Note: Input is already trimmed by TrimStrings() before validation
func validateIFSC(fl validator.FieldLevel) bool {
	ifsc := fl.Field().String()
	// IFSC format: 4 bank code letters + 1 zero + 6 branch code alphanumeric
	ifscRegex := regexp.MustCompile(`^[A-Z]{4}0[A-Z0-9]{6}$`)
	return ifscRegex.MatchString(ifsc)
}

// validateBusinessName validates business name format
// Allows: Letters, numbers, spaces, and .,&()'-/@#
// Note: Input is already trimmed by TrimStrings() before validation
func validateBusinessName(fl validator.FieldLevel) bool {
	name := fl.Field().String()
	// Business name: alphanumeric + spaces + extended punctuation
	nameRegex := regexp.MustCompile(`^[A-Za-z0-9\s.,&()'\-/@#]+$`)
	return nameRegex.MatchString(name)
}

// validatePersonName validates person/user name format
// Allows: Letters, spaces, and basic punctuation (.-')
// Note: Input is already trimmed by TrimStrings() before validation
func validatePersonName(fl validator.FieldLevel) bool {
	name := fl.Field().String()
	// Person name: letters + spaces + basic punctuation
	nameRegex := regexp.MustCompile(`^[A-Za-z\s.\-']+$`)
	return nameRegex.MatchString(name)
}

// validateSafeText validates text fields for general safe input
// Prevents common injection patterns while allowing most characters
// Blocks: <, >, NULL bytes, and control characters (except newline/tab/CR)
// Note: Input is already trimmed by TrimStrings() before validation
func validateSafeText(fl validator.FieldLevel) bool {
	text := fl.Field().String()
	// Block angle brackets, NULL bytes, and control characters
	for _, char := range text {
		if char == '<' || char == '>' || char == 0 || (char < 32 && char != '\n' && char != '\r' && char != '\t') {
			return false
		}
	}
	return true
}

// validateOAuthCode validates OAuth2 authorization code format
// Allows: Alphanumeric characters and ._- (common in OAuth codes)
// Length: 20-512 characters (RFC 6749 compliant)
// Note: Input is already trimmed by TrimStrings() before validation
func validateOAuthCode(fl validator.FieldLevel) bool {
	code := fl.Field().String()
	if len(code) < 20 || len(code) > 512 {
		return false
	}
	codeRegex := regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	return codeRegex.MatchString(code)
}

// TrimStrings trims all string fields in a struct (recursively)
// This modifies the struct in place to remove leading/trailing whitespace
func TrimStrings(data interface{}) {
	val := reflect.ValueOf(data)

	// If it's a pointer, get the underlying value
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	// Only process structs
	if val.Kind() != reflect.Struct {
		return
	}

	// Iterate through all fields
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)

		// Skip unexported fields
		if !field.CanSet() {
			continue
		}

		switch field.Kind() {
		case reflect.String:
			// Trim string fields
			trimmed := strings.TrimSpace(field.String())
			field.SetString(trimmed)

		case reflect.Struct:
			// Recursively trim nested structs
			TrimStrings(field.Addr().Interface())

		case reflect.Ptr:
			// Handle pointer to struct
			if !field.IsNil() && field.Elem().Kind() == reflect.Struct {
				TrimStrings(field.Interface())
			}
		}
	}
}

// BindAndValidate is a helper function to bind, trim, and validate request data
func BindAndValidate(c echo.Context, req interface{}) error {
	// Bind the request
	if err := c.Bind(req); err != nil {
		return &echo.HTTPError{
			Code:    400,
			Message: "Invalid request payload",
		}
	}

	// Trim all string fields
	TrimStrings(req)

	// Validate the request
	if err := c.Validate(req); err != nil {
		if ve, ok := err.(*ValidationError); ok {
			return c.JSON(400, map[string]interface{}{
				"message": "Validation failed",
				"errors":  ve.Errors,
				"data":    nil,
			})
		}
		return c.JSON(400, map[string]interface{}{
			"message": "Validation failed",
			"data":    nil,
		})
	}

	return nil
}
