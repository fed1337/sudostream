package httpapi

import (
	"net/http"
	"sudoStream/internal/auth"
	"sudoStream/internal/auth/ginjwt"
	"sudoStream/internal/observability"

	jwtmw "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
)

const userIdentityKey = "authUser"

func jwtMiddleware(authService *auth.Service) gin.HandlerFunc {
	issuer, ok := authService.Issuer().(*ginjwt.Issuer)
	if !ok || issuer == nil {
		return func(c *gin.Context) {
			c.AbortWithStatusJSON(
				http.StatusUnauthorized,
				ErrorResponse{Error: authRequiredMessage},
			)
		}
	}

	return func(c *gin.Context) {
		issuer.Middleware()(c)
		if c.IsAborted() {
			return
		}

		claims := jwtmw.ExtractClaims(c)
		if sessionID, ok := claims[auth.ClaimSessionID].(string); ok && sessionID != "" {
			c.Set(sessionContextKey, sessionID)
		}

		if identity, exists := c.Get(userIdentityKey); exists {
			if user, ok := identity.(auth.PublicUser); ok {
				c.Set(userContextKey, user)
				c.Set(observability.UserIDKey, user.ID)
			}
		}
	}
}

func optionalAuth(authService *auth.Service) gin.HandlerFunc {
	issuer, ok := authService.Issuer().(*ginjwt.Issuer)
	if !ok || issuer == nil {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		user, sessionID, authenticated := issuer.TryAuthenticate(c)
		if authenticated {
			c.Set(userContextKey, user)
			c.Set(sessionContextKey, sessionID)
			c.Set(observability.UserIDKey, user.ID)
		}

		c.Next()
	}
}
