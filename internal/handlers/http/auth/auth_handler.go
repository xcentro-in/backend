package auth

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/shared/logger"
)

func placeHolderFn(c echo.Context) error { return c.String(200, "This is a placeholder function") }

// create two functions one for login and one for signup
type AuthHandler struct {
	userService user.UserService
	authService auth.AuthService
}

func NewAuthHandler(userService user.UserService, authService auth.AuthService) *AuthHandler {
	return &AuthHandler{
		userService: userService,
		authService: authService,
	}
}

func RegisterAuthRoutes(e *echo.Group, userService user.UserService, authService auth.AuthService) {
	authHandler := NewAuthHandler(userService, authService)

	authGroup := e.Group("/auth")
	/**
	  can add middleware here for auth group and its subgroups
	**/
	authGroup.POST("/login", authHandler.HandleLogin)
	authGroup.POST("/signup", authHandler.HandleSignup)

	// Sub group is prefixed with /auth
	oauthGroup := authGroup.Group("/Oauth")
	/**
	  can add middleware here for Oauth group and its subgroups
	**/
	oauthGroup.GET("/redirecturl", placeHolderFn)
	oauthGroup.GET("/callback", placeHolderFn)
}

func (ah AuthHandler) HandleSignup(c echo.Context) error {
	var signupReq auth.AuthPayload

	err := c.Bind(&signupReq)
	if err != nil {
		logger.Errorf("Invalid input %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid input provided", "data": nil})
	}

	if signupReq.Email == "" || signupReq.Password == "" {
		logger.Error("Email or password missing")
		return c.JSON(400, map[string]any{"message": "Email or password missing", "data": nil})
	}

	hashedPassword, err := ah.authService.HashPassword(signupReq.Password)
	if err != nil {
		logger.Errorf("Failed to hash password %v", err)
		return c.JSON(500, map[string]any{"message": "Failed to process password", "data": nil})
	}
	signupReq.Password = hashedPassword

	user, err := ah.userService.CreateUser(c.Request().Context(), signupReq)
	if err != nil {
		logger.Errorf("Failed to create user %v", err)
		return c.JSON(500, map[string]any{"message": "Failed to create user", "data": nil})
	}

	token, err := ah.authService.CreateAuthJWT(user.Id)
	if err != nil {
		logger.Errorf("Failed to create token %v", err)
		return c.JSON(500, map[string]any{"message": "Failed to create token", "data": nil})
	}

	rclient := ah.authService.GetRedisClient()
	rclient.Client.HSet(c.Request().Context(), "xcentro:"+user.Id, "token", token, "isrevoked", "false")

	return c.JSON(201, map[string]any{"message": "User created successfully", "data": map[string]string{"token": token}})

}

func (ah AuthHandler) HandleLogin(c echo.Context) error {
	var loginReq auth.AuthPayload
	err := c.Bind(&loginReq)
	if err != nil {
		logger.Errorf("Invalid input %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid input provided", "data": nil})
	}

	if loginReq.Email == "" || loginReq.Password == "" {
		logger.Error("Email or password missing")
		return c.JSON(400, map[string]any{"message": "Email or password missing", "data": nil})
	}

	user, err := ah.userService.GetUserByEmail(c.Request().Context(), loginReq.Email)
	if err != nil {
		logger.Errorf("Failed to get user by email %v", err)
		return c.JSON(500, map[string]any{"message": "Failed to get user by email", "data": nil})
	}

	if user == nil || ah.authService.ComparePassword(user.PwdHash, loginReq.Password) != nil {
		logger.Error("Invalid email or password")
		return c.JSON(401, map[string]any{"message": "Invalid email or password", "data": nil})
	}

	token, err := ah.authService.CreateAuthJWT(user.Id)
	if err != nil {
		logger.Errorf("Failed to create token %v", err)
		return c.JSON(500, map[string]any{"message": "Failed to create token", "data": nil})
	}

	rclient := ah.authService.GetRedisClient()
	rclient.Client.HSet(c.Request().Context(), "xcentro:"+user.Id, "token", token, "isrevoked", "false")

	return c.JSON(200, map[string]any{"message": "Login successful", "data": map[string]string{"token": token}})
}
