package types

import "fmt"

type ReplyError struct {
	Msg string
}

func (err ReplyError) Error() string {
	return err.Msg
}

// Business onboarding errors
type BusinessError struct {
	Code    string
	Message string
	Details string
}

func (e *BusinessError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s: %s", e.Message, e.Details)
	}
	return e.Message
}

// Error codes
const (
	ErrCodeUserAlreadyAssociated = "USER_ALREADY_ASSOCIATED"
	ErrCodeBusinessAlreadyExists = "BUSINESS_ALREADY_EXISTS"
	ErrCodeUnauthorizedKYB       = "UNAUTHORIZED_KYB"
	ErrCodeKYBAlreadyCompleted   = "KYB_ALREADY_COMPLETED"
	ErrCodeInvalidBusinessType   = "INVALID_BUSINESS_TYPE"
	ErrCodeBusinessNotFound      = "BUSINESS_NOT_FOUND"
	ErrCodeKYBNotInitiated       = "KYB_NOT_INITIATED"
	ErrCodeInvalidInput          = "INVALID_INPUT"
)

// Predefined errors
var (
	ErrUserAlreadyAssociated = &BusinessError{
		Code:    ErrCodeUserAlreadyAssociated,
		Message: "User is already associated with a business",
	}

	ErrUnauthorizedKYB = &BusinessError{
		Code:    ErrCodeUnauthorizedKYB,
		Message: "User is not authorized to update KYB for this business",
	}

	ErrKYBAlreadyCompleted = &BusinessError{
		Code:    ErrCodeKYBAlreadyCompleted,
		Message: "KYB has already been completed for this business",
	}

	ErrKYBNotInitiated = &BusinessError{
		Code:    ErrCodeKYBNotInitiated,
		Message: "KYB has not been initiated for this business",
	}

	ErrBusinessNotFound = &BusinessError{
		Code:    ErrCodeBusinessNotFound,
		Message: "Business not found",
	}
)

// Constructor functions
func NewBusinessAlreadyExistsError(gstin string) *BusinessError {
	return &BusinessError{
		Code:    ErrCodeBusinessAlreadyExists,
		Message: "Business already exists",
		Details: fmt.Sprintf("GSTIN %s is already registered", gstin),
	}
}

func NewInvalidBusinessTypeError(businessType string) *BusinessError {
	return &BusinessError{
		Code:    ErrCodeInvalidBusinessType,
		Message: "Invalid business type",
		Details: fmt.Sprintf("Business type '%s' is not valid", businessType),
	}
}

func NewInvalidInputError(field string, details string) *BusinessError {
	return &BusinessError{
		Code:    ErrCodeInvalidInput,
		Message: fmt.Sprintf("Invalid input for %s", field),
		Details: details,
	}
}

// User onboarding errors
type UserError struct {
	Code    string
	Message string
	Details string
}

func (e *UserError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s: %s", e.Message, e.Details)
	}
	return e.Message
}

// User error codes
const (
	ErrCodeKYCAlreadyCompleted = "KYC_ALREADY_COMPLETED"
	ErrCodeKYCNotStarted       = "KYC_NOT_STARTED"
	ErrCodeInvalidKYCInput     = "INVALID_KYC_INPUT"
	ErrCodeUnauthorizedKYC     = "UNAUTHORIZED_KYC"
	ErrCodeVerificationFailed  = "VERIFICATION_FAILED"
	ErrCodeUserNotFound        = "USER_NOT_FOUND"
	ErrCodeAadhaarNotInitiated = "AADHAAR_NOT_INITIATED"
)

// Predefined user errors
var (
	ErrKYCAlreadyCompleted = &UserError{
		Code:    ErrCodeKYCAlreadyCompleted,
		Message: "KYC has already been completed",
	}

	ErrKYCNotStarted = &UserError{
		Code:    ErrCodeKYCNotStarted,
		Message: "KYC has not been started yet",
	}

	ErrUnauthorizedKYC = &UserError{
		Code:    ErrCodeUnauthorizedKYC,
		Message: "User is not authorized to perform KYC for this business",
	}

	ErrUserNotFound = &UserError{
		Code:    ErrCodeUserNotFound,
		Message: "User not found",
	}

	ErrAadhaarNotInitiated = &UserError{
		Code:    ErrCodeAadhaarNotInitiated,
		Message: "Aadhaar verification has not been initiated",
	}
)

// Constructor functions for user errors
func NewVerificationFailedError(verificationType string, details string) *UserError {
	return &UserError{
		Code:    ErrCodeVerificationFailed,
		Message: fmt.Sprintf("%s verification failed", verificationType),
		Details: details,
	}
}

func NewInvalidKYCInputError(field string, details string) *UserError {
	return &UserError{
		Code:    ErrCodeInvalidKYCInput,
		Message: fmt.Sprintf("Invalid input for %s", field),
		Details: details,
	}
}
