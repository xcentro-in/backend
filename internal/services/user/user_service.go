package user

import (
	"fmt"

	"xcentro.in/internal/shared/logger"
)

type UserService struct {
	// Add necessary fields here, e.g., database connection, logger, etc.
}

func NewUserService() *UserService {
	return &UserService{
		// Initialize fields here.
	}
}

func (s *UserService) CreateUser(name, email string) string {
	logger.GetDefault().Info("Creating user: Name=%s, Email=%s", name, email)
	return fmt.Sprintf("Creating user: Name=%s, Email=%s", name, email)
}
