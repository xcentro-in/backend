package payout

import "errors"

// Domain errors for payout operations
var (
	// Provider mapping errors
	ErrMappingNotFound      = errors.New("provider mapping not found")
	ErrMappingAlreadyExists = errors.New("provider mapping already exists")
	ErrNoActiveMapping      = errors.New("no active provider mapping found")
	ErrPayeeNotOnboarded    = errors.New("payee not onboarded with provider")

	// Transfer errors
	ErrTransferNotFound       = errors.New("transfer request not found")
	ErrTransferAlreadyExists  = errors.New("transfer request already exists for voucher")
	ErrTransferNotPending     = errors.New("transfer is not in pending status")
	ErrTransferNotApproved    = errors.New("transfer not approved")
	ErrApprovalNotRequired    = errors.New("transfer does not require approval")
	ErrTransferAlreadyApproved = errors.New("transfer already approved")

	// Validation errors
	ErrInvalidAmount        = errors.New("invalid transfer amount")
	ErrInvalidBeneficiaryID = errors.New("invalid provider beneficiary ID")
	ErrInvalidProvider      = errors.New("invalid provider type")
)
