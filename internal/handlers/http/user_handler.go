package httphandler

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	userintf "xcentro.in/internal/core/user"
	"xcentro.in/internal/services/user"
)

func RegisterRouters(e *echo.Echo) {
	group := e.Group("/user")
	e.Pre(middleware.AddTrailingSlash())
	group.GET("/", func(c echo.Context) error {
		var service userintf.UserService = user.NewUserService()
		// TODO: Initialize service as needed
		return c.JSON(200, map[string]string{"message": service.CreateUser("darahas", "darahask12@gmail.com")})
	})
}
