package postgres

import (
	"time"

	"gorm.io/datatypes"
)

type userModel struct {
	ID                 string         `gorm:"column:id;primaryKey;type:uuid"`
	Email              string         `gorm:"column:email"`
	PasswordHash       string         `gorm:"column:password_hash"`
	Role               string         `gorm:"column:role"`
	Enabled            bool           `gorm:"column:enabled"`
	EmailVerifiedAt    *time.Time     `gorm:"column:email_verified_at"`
	MustChangePassword bool           `gorm:"column:must_change_password"`
	CreatedAt          time.Time      `gorm:"column:created_at"`
	UpdatedAt          time.Time      `gorm:"column:updated_at"`
	LastLoginAt        *time.Time     `gorm:"column:last_login_at"`
	AudioLanguagePrefs datatypes.JSON `gorm:"column:audio_language_prefs"`
}

func (userModel) TableName() string {
	return "users"
}

type sessionModel struct {
	ID        string    `gorm:"column:id;primaryKey;type:uuid"`
	UserID    string    `gorm:"column:user_id;type:uuid"`
	TokenHash string    `gorm:"column:token_hash"`
	ExpiresAt time.Time `gorm:"column:expires_at"`
	CreatedAt time.Time `gorm:"column:created_at"`
	IP        *string   `gorm:"column:ip"`
	UserAgent *string   `gorm:"column:user_agent"`
}

func (sessionModel) TableName() string {
	return "sessions"
}

type settingsModel struct {
	Key       string         `gorm:"column:key;primaryKey"`
	Value     datatypes.JSON `gorm:"column:value"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
}

func (settingsModel) TableName() string {
	return "settings"
}

type authTokenModel struct {
	ID        string     `gorm:"column:id;primaryKey;type:uuid"`
	UserID    *string    `gorm:"column:user_id;type:uuid"`
	Email     string     `gorm:"column:email"`
	Purpose   string     `gorm:"column:purpose"`
	TokenHash string     `gorm:"column:token_hash"`
	Role      *string    `gorm:"column:role"`
	InvitedBy *string    `gorm:"column:invited_by;type:uuid"`
	ExpiresAt time.Time  `gorm:"column:expires_at"`
	UsedAt    *time.Time `gorm:"column:used_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
}

func (authTokenModel) TableName() string {
	return "auth_tokens"
}

type userTOTPModel struct {
	UserID          string         `gorm:"column:user_id;primaryKey;type:uuid"`
	SecretEncrypted string         `gorm:"column:secret_encrypted"`
	EnabledAt       *time.Time     `gorm:"column:enabled_at"`
	BackupCodesRaw  datatypes.JSON `gorm:"column:backup_codes_hash"`
}

func (userTOTPModel) TableName() string {
	return "user_totp"
}
