package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

func RequireAdmin(adminKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if adminKey == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": "Admin authentication is not configured.",
			})
		}

		requestKey := c.GetHeader("X-Admin-Key")
		if requestKey == "" ||
			subtle.ConstantTimeCompare(
				[]byte(requestKey),
				[]byte(adminKey),
			) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid or missing admin key.",
			})
			return
		}
		c.Next()
	}
}
