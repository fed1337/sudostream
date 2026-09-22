package auth

// SessionIdentity is passed to the JWT issuer when creating or refreshing tokens.
type SessionIdentity struct {
	SessionID string
	UserID    string
	User      PublicUser
	Meta      SessionMeta
}
