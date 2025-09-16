package shared

type KYCStatus uint8

type UserRole uint8

const (
	Pending KYCStatus = iota
	InProgress
	Completed
)

const (
	Owner UserRole = iota
	Admin
	Finance
	Support
)
