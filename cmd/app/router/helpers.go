package router

import (
	"github.com/labstack/echo/v4"
	"net/http"
)

type Header struct {
	Name  string
	Value string
}

func defaultHeaders(headers []Header, filter func(r *http.Request) bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if filter(c.Request()) {
				for _, header := range headers {
					c.Response().Header().Set(header.Name, header.Value)
				}
			}
			return next(c)
		}
	}
}
