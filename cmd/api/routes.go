package main

import "github.com/labstack/echo/v4"

func placeHolderFn(c echo.Context) error { return c.String(200, "This is a placeholder function") }

func registerV1AuthRoutes(e *echo.Group) {
	authGroup := e.Group("/auth")
	/**
	  can add middleware here for auth group and its subgroups
	**/

	authGroup.GET("/login", placeHolderFn)
	authGroup.POST("/signup", placeHolderFn)

	// Sub group is prefixed with /auth
	oauthGroup := authGroup.Group("/Oauth")
	/**
	  can add middleware here for Oauth group and its subgroups
	**/
	oauthGroup.GET("/redirecturl", placeHolderFn)
	oauthGroup.GET("/callback", placeHolderFn)
}

func registerV1Routes(e *echo.Echo) {
	v1 := e.Group("/v1")

	// Health Check Endpoint
	v1.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{
			"status": "ok",
		})
	})

	// Authentication Routes
	registerV1AuthRoutes(v1)
	// User Routes
	// Buiseness Routes
}

func RegisterRouters(e *echo.Echo) {
	registerV1Routes(e)
}
