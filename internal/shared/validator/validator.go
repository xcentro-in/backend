package validator

import (
	"fmt"
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

	return &CustomValidator{validator: v}
}

// Validate implements echo.Validator interface
func (cv *CustomValidator) Validate(i interface{}) error {
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
	default:
		return fmt.Sprintf("%s is invalid", field)
	}
}

// validatePAN validates Indian PAN format: AAAAA9999A
func validatePAN(fl validator.FieldLevel) bool {
	pan := strings.TrimSpace(fl.Field().String())
	panRegex := regexp.MustCompile(`^[A-Z]{5}[0-9]{4}[A-Z]{1}$`)
	return panRegex.MatchString(pan)
}

// validateGSTIN validates Indian GSTIN format: 22AAAAA9999A1Z5
func validateGSTIN(fl validator.FieldLevel) bool {
	gstin := strings.TrimSpace(fl.Field().String())
	gstinRegex := regexp.MustCompile(`^[0-9]{2}[A-Z]{5}[0-9]{4}[A-Z]{1}[1-9A-Z]{1}Z[0-9A-Z]{1}$`)
	return gstinRegex.MatchString(gstin)
}

// validateAadhaar validates Indian Aadhaar format: 2345 6789 0123 (with optional spaces)
// Must start with 2-9 (not 0 or 1), exactly 12 digits
func validateAadhaar(fl validator.FieldLevel) bool {
	aadhaar := strings.TrimSpace(fl.Field().String())
	// Allow optional spaces: 2345 6789 0123 or 234567890123
	aadhaarRegex := regexp.MustCompile(`^[2-9]{1}[0-9]{3}\s?[0-9]{4}\s?[0-9]{4}$`)
	return aadhaarRegex.MatchString(aadhaar)
}

// validateCIN validates Indian Company Identification Number format: L12345KA2010PLC012345
// Starts with L or U, 21 characters total
func validateCIN(fl validator.FieldLevel) bool {
	cin := strings.TrimSpace(fl.Field().String())
	cinRegex := regexp.MustCompile(`^[LU][0-9]{5}[A-Z]{2}[0-9]{4}[A-Z]{3}[0-9]{6}$`)
	return cinRegex.MatchString(cin)
}

// validateIndianPhone validates 10-digit Indian phone number
func validateIndianPhone(fl validator.FieldLevel) bool {
	phone := strings.TrimSpace(fl.Field().String())
	// Must be exactly 10 digits
	phoneRegex := regexp.MustCompile(`^[0-9]{10}$`)
	return phoneRegex.MatchString(phone)
}

// validateStrongPassword validates password strength using existing password utils
func validateStrongPassword(fl validator.FieldLevel) bool {
	password := fl.Field().String()
	return security.ValidatePasswordStrength(password) == nil
}

// BindAndValidate is a helper function to bind and validate request data
func BindAndValidate(c echo.Context, req interface{}) error {
	// Bind the request
	if err := c.Bind(req); err != nil {
		return &echo.HTTPError{
			Code:    400,
			Message: "Invalid request payload",
		}
	}

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
