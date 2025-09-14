package user

type UserService interface {
	// Define user-related methods here
	CreateUser(name, email string) string
}
