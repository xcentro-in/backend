package httphandler

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	userintf "xcentro.in/internal/core/user"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/utils"
)

var signingKey = []byte("TESTToken")

func RegisterRouters(e *echo.Echo, userService userintf.UserService) {
	userG := e.Group("/auth")
	userG.POST("/login", func(c echo.Context) error {
		// Use the injected service instead of creating a new one

		// Define a struct to bind JSON request
		type LoginRequest struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}

		var req LoginRequest
		if err := c.Bind(&req); err != nil {
			logger.Error("failed to bind request: %v", err)
			return c.JSON(400, map[string]string{"error": "Invalid request format"})
		}

		// Validate required fields
		if req.Email == "" || req.Password == "" {
			return c.JSON(400, map[string]string{"error": "Email and password are required"})
		}

		userid := fmt.Sprintf("user:%spass:%s", req.Email, req.Password)

		claim := utils.ClaimsWithUserId(userid)
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claim)
		stoken, err := token.SignedString(signingKey)

		if err != nil {
			logger.Error("token signing failed: %v", err)
			return c.JSON(500, "Internal Server Error")
		}

		return c.JSON(200, map[string]interface{}{"message": "Login successful", "token": stoken})
	})

	protectedG := e.Group("/user")
	protectedG.Use(echojwt.WithConfig(echojwt.Config{
		ContextKey: "auth",
		NewClaimsFunc: func(c echo.Context) jwt.Claims {
			return &utils.UserJWTClaims{}
		},
		KeyFunc: func(t *jwt.Token) (interface{}, error) {
			return signingKey, nil
		},
	}))

	protectedG.GET("/profile", func(e echo.Context) error {
		var token *jwt.Token = e.Get("auth").(*jwt.Token)
		return e.JSON(200, token.Claims.(*utils.UserJWTClaims))
	})
}
