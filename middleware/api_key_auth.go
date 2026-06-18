package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ApiKeyAuth validates X-Api-Key using callbacks so it can be reused by integration modules.
func ApiKeyAuth(enabled func() bool, getKey func() string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if enabled == nil || getKey == nil || !enabled() {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		expected := strings.TrimSpace(getKey())
		if expected == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		got := strings.TrimSpace(c.GetHeader("X-Api-Key"))
		if got == "" || got != expected {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}
