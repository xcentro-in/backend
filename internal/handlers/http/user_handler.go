package http

import (
	"github.com/labstack/echo/v4"
	intfuser "xcentro.in/internal/core/services/user"
)

func RegisterRouters(e *echo.Echo) {
	group := e.Group("/user")
	group.GET("/", func(c echo.Context) error {

		var service intfuser.UserService = new.NewUserService()
		// TODO: Initialize service as needed

		return c.JSON(200, map[string]string{"message": "success"})
	})

}
