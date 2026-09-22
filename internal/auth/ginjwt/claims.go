package ginjwt

import (
	"sudoStream/internal/auth"

	"github.com/golang-jwt/jwt/v5"
)

func sessionClaims(data any) jwt.MapClaims {
	identity, ok := data.(auth.SessionIdentity)
	if !ok {
		return jwt.MapClaims{}
	}

	return jwt.MapClaims{
		auth.ClaimSessionID:      identity.SessionID,
		auth.ClaimUserID:         identity.UserID,
		auth.ClaimEmail:          identity.User.Email,
		auth.ClaimRole:           identity.User.Role,
		auth.ClaimMustChangePass: identity.User.MustChangePassword,
	}
}
