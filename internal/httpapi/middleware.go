package httpapi

import (
	"net/http"
	"sudoStream/internal/auth"

	"github.com/gin-gonic/gin"
)

func requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := currentUser(c)
		if !ok {
			c.AbortWithStatusJSON(
				http.StatusUnauthorized,
				ErrorResponse{Error: authRequiredMessage},
			)

			return
		}
		if user.Role != auth.RoleAdmin {
			c.AbortWithStatusJSON(
				http.StatusForbidden,
				ErrorResponse{Error: "admin access required"},
			)

			return
		}

		c.Next()
	}
}
