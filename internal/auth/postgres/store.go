// Package postgres implements auth persistence with PostgreSQL.
//
//nolint:funcorder // helpers are grouped near their call sites for readability.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sudoStream/internal/auth"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	errSettingsNotFound        = errors.New("settings not found")
	errAuthTokenNotFound       = errors.New("auth token not found")
	errUnsupportedLookupColumn = errors.New("unsupported user lookup column")
	columnUpdatedAt            = "updated_at"
)

// Store persists auth data in PostgreSQL.
type Store struct {
	db *gorm.DB
}

// NewStore constructs a PostgreSQL auth store.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// CountSessions returns the number of refresh sessions currently stored.
func (s *Store) CountSessions(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&sessionModel{}).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count sessions: %w", err)
	}

	return count, nil
}

// CountUsers returns the number of registered users.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var count int64

	err := s.db.WithContext(ctx).Model(&userModel{}).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}

	return int(count), nil
}

// CountAdmins returns the number of admin users.
func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var count int64

	err := s.db.WithContext(ctx).
		Model(&userModel{}).
		Where("role = ?", auth.RoleAdmin).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count admins: %w", err)
	}

	return int(count), nil
}

// CreateUser inserts a new user with the given password hash.
func (s *Store) CreateUser(ctx context.Context, user auth.User, passwordHash string) error {
	model := userModel{
		Email:              user.Email,
		PasswordHash:       passwordHash,
		Role:               user.Role,
		Enabled:            user.Enabled,
		MustChangePassword: user.MustChangePassword,
		AudioLanguagePrefs: datatypes.JSON("[]"),
	}

	err := s.db.WithContext(ctx).
		Omit("ID", "CreatedAt", "UpdatedAt", "LastLoginAt", "EmailVerifiedAt").
		Create(&model).Error
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}

	return nil
}

// GetUserByEmail loads a user and password hash by email.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*auth.User, string, error) {
	return s.getUserByEmail(ctx, email, true)
}

// GetUserByID loads a user by primary key.
func (s *Store) GetUserByID(ctx context.Context, id string) (*auth.User, error) {
	user, _, err := s.getUserByID(ctx, id, false)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// GetUserWithPasswordByID loads a user and password hash by ID.
func (s *Store) GetUserWithPasswordByID(
	ctx context.Context,
	id string,
) (*auth.User, string, error) {
	return s.getUserByID(ctx, id, true)
}

// ListUsers returns all users for admin listing.
func (s *Store) ListUsers(ctx context.Context) ([]auth.AdminUser, error) {
	type adminRow struct {
		ID                 string
		Email              string
		Role               string
		Enabled            bool
		EmailVerifiedAt    *time.Time
		MustChangePassword bool
		LastLoginAt        *time.Time
		CreatedAt          time.Time
		Has2FA             bool `gorm:"column:has_2fa"`
	}

	var rows []adminRow

	err := s.db.WithContext(ctx).
		Table("users u").
		Select(`
			u.id, u.email, u.role, u.enabled, u.email_verified_at,
			u.must_change_password, u.last_login_at, u.created_at,
			(t.enabled_at IS NOT NULL) AS has_2fa`).
		Joins("LEFT JOIN user_totp t ON t.user_id = u.id").
		Order("u.created_at ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	users := make([]auth.AdminUser, 0, len(rows))
	for _, row := range rows {
		users = append(users, auth.AdminUser{
			ID:                 row.ID,
			Email:              row.Email,
			Role:               row.Role,
			Enabled:            row.Enabled,
			EmailVerified:      row.EmailVerifiedAt != nil,
			MustChangePassword: row.MustChangePassword,
			LastLoginAt:        row.LastLoginAt,
			CreatedAt:          row.CreatedAt,
			Has2FA:             row.Has2FA,
		})
	}

	return users, nil
}

// UpdateUser applies admin patches.
func (s *Store) UpdateUser(ctx context.Context, userID string, patch auth.UserPatch) error {
	if patch.Enabled != nil {
		result := s.db.WithContext(ctx).
			Model(&userModel{}).
			Where("id = ?", userID).
			Updates(map[string]any{
				"enabled":       *patch.Enabled,
				columnUpdatedAt: gorm.Expr("NOW()"),
			})
		if result.Error != nil {
			return fmt.Errorf("update enabled: %w", result.Error)
		}
	}

	if patch.Role != nil {
		result := s.db.WithContext(ctx).
			Model(&userModel{}).
			Where("id = ?", userID).
			Updates(map[string]any{
				"role":          *patch.Role,
				columnUpdatedAt: gorm.Expr("NOW()"),
			})
		if result.Error != nil {
			return fmt.Errorf("update role: %w", result.Error)
		}
	}

	return nil
}

// UpdateLastLogin sets last_login_at for a user.
func (s *Store) UpdateLastLogin(ctx context.Context, userID string, at time.Time) error {
	result := s.db.WithContext(ctx).
		Model(&userModel{}).
		Where("id = ?", userID).
		Updates(map[string]any{
			"last_login_at": at,
			columnUpdatedAt: gorm.Expr("NOW()"),
		})
	if result.Error != nil {
		return fmt.Errorf("update last login: %w", result.Error)
	}

	return nil
}

// UpdatePasswordHash updates a user's password hash.
func (s *Store) UpdatePasswordHash(
	ctx context.Context,
	userID, passwordHash string,
	mustChangePassword bool,
) error {
	result := s.db.WithContext(ctx).
		Model(&userModel{}).
		Where("id = ?", userID).
		Updates(map[string]any{
			"password_hash":        passwordHash,
			"must_change_password": mustChangePassword,
			columnUpdatedAt:        gorm.Expr("NOW()"),
		})
	if result.Error != nil {
		return fmt.Errorf("update password hash: %w", result.Error)
	}

	return nil
}

// UpdateUserEmail sets a user's email and optionally clears verification.
func (s *Store) UpdateUserEmail(
	ctx context.Context,
	userID, email string,
	clearVerified bool,
) error {
	updates := map[string]any{
		"email":         email,
		columnUpdatedAt: gorm.Expr("NOW()"),
	}
	if clearVerified {
		updates["email_verified_at"] = nil
	}

	result := s.db.WithContext(ctx).
		Model(&userModel{}).
		Where("id = ?", userID).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update user email: %w", result.Error)
	}

	return nil
}

// EnableUser activates an account and optionally records email verification.
func (s *Store) EnableUser(ctx context.Context, userID string, markEmailVerified bool) error {
	updates := map[string]any{
		"enabled":       true,
		columnUpdatedAt: gorm.Expr("NOW()"),
	}
	if markEmailVerified {
		updates["email_verified_at"] = gorm.Expr("COALESCE(email_verified_at, NOW())")
	}

	result := s.db.WithContext(ctx).
		Model(&userModel{}).
		Where("id = ?", userID).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("enable user: %w", result.Error)
	}

	return nil
}

// ClearEmailVerified clears email_verified_at without changing enabled.
func (s *Store) ClearEmailVerified(ctx context.Context, userID string) error {
	result := s.db.WithContext(ctx).
		Model(&userModel{}).
		Where("id = ?", userID).
		Updates(map[string]any{
			"email_verified_at": nil,
			columnUpdatedAt:     gorm.Expr("NOW()"),
		})
	if result.Error != nil {
		return fmt.Errorf("clear email verified: %w", result.Error)
	}

	return nil
}

// DeleteUser hard-deletes a user row (cascades sessions/tokens/grants via FK).
func (s *Store) DeleteUser(ctx context.Context, userID string) error {
	result := s.db.WithContext(ctx).
		Where("id = ?", userID).
		Delete(&userModel{})
	if result.Error != nil {
		return fmt.Errorf("delete user: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return auth.ErrManagedUserNotFound
	}

	return nil
}

// HasUnusedAuthToken reports whether an unused, unexpired token exists.
func (s *Store) HasUnusedAuthToken(ctx context.Context, email, purpose string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).
		Model(&authTokenModel{}).
		Where(
			"email = ? AND purpose = ? AND used_at IS NULL AND expires_at > NOW()",
			strings.ToLower(strings.TrimSpace(email)),
			purpose,
		).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("count auth tokens: %w", err)
	}

	return count > 0, nil
}

// GetUnusedAuthTokenByUser loads an unused, unexpired token for a user and purpose.
func (s *Store) GetUnusedAuthTokenByUser(
	ctx context.Context,
	userID, purpose string,
) (*auth.Token, error) {
	var model authTokenModel
	err := s.db.WithContext(ctx).
		Where(
			"user_id = ? AND purpose = ? AND used_at IS NULL AND expires_at > NOW()",
			userID,
			purpose,
		).
		Order("created_at DESC").
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errAuthTokenNotFound
		}

		return nil, fmt.Errorf("select auth token: %w", err)
	}

	return tokenFromModel(model), nil
}

// GetSettings loads auth policy settings.
func (s *Store) GetSettings(ctx context.Context) (auth.Settings, error) {
	raw, err := s.GetSettingValue(ctx, authSettingsKey())
	if err != nil {
		if errors.Is(err, errSettingsNotFound) {
			return auth.Settings{}, errSettingsNotFound
		}

		return auth.Settings{}, err
	}

	var settings auth.Settings
	err = json.Unmarshal(raw, &settings)
	if err != nil {
		return auth.Settings{}, fmt.Errorf("decode settings: %w", err)
	}

	return settings, nil
}

// SaveSettings persists auth policy settings.
func (s *Store) SaveSettings(ctx context.Context, settings auth.Settings) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}

	return s.SaveSettingValue(ctx, authSettingsKey(), raw)
}

// GetSettingValue loads a raw JSON document from the settings table.
func (s *Store) GetSettingValue(ctx context.Context, key string) ([]byte, error) {
	var model settingsModel

	err := s.db.WithContext(ctx).
		Where("key = ?", key).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errSettingsNotFound
		}

		return nil, fmt.Errorf("select settings %q: %w", key, err)
	}

	return []byte(model.Value), nil
}

// SaveSettingValue upserts a raw JSON document in the settings table.
func (s *Store) SaveSettingValue(ctx context.Context, key string, value []byte) error {
	model := settingsModel{
		Key:   key,
		Value: datatypes.JSON(value),
	}

	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", columnUpdatedAt}),
	}).Create(&model).Error
	if err != nil {
		return fmt.Errorf("upsert settings %q: %w", key, err)
	}

	return nil
}

// CreateAuthToken inserts a token row.
func (s *Store) CreateAuthToken(ctx context.Context, token auth.Token) error {
	model := authTokenModel{
		UserID:    nullStringPtr(token.UserID),
		Email:     token.Email,
		Purpose:   token.Purpose,
		TokenHash: token.TokenHash,
		Role:      nullStringPtr(token.Role),
		InvitedBy: nullStringPtr(token.InvitedBy),
		ExpiresAt: token.ExpiresAt,
	}

	err := s.db.WithContext(ctx).
		Omit("ID", "CreatedAt", "UsedAt").
		Create(&model).Error
	if err != nil {
		return fmt.Errorf("insert auth token: %w", err)
	}

	return nil
}

// GetAuthTokenByHash loads a token by hash and purpose.
func (s *Store) GetAuthTokenByHash(
	ctx context.Context,
	tokenHash, purpose string,
) (*auth.Token, error) {
	var model authTokenModel

	err := s.db.WithContext(ctx).
		Where("token_hash = ? AND purpose = ?", tokenHash, purpose).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("auth token not found: %w", err)
		}

		return nil, fmt.Errorf("select auth token: %w", err)
	}

	return tokenFromModel(model), nil
}

// MarkAuthTokenUsed marks a token consumed.
func (s *Store) MarkAuthTokenUsed(ctx context.Context, tokenID string) error {
	result := s.db.WithContext(ctx).
		Model(&authTokenModel{}).
		Where("id = ?", tokenID).
		Update("used_at", gorm.Expr("NOW()"))
	if result.Error != nil {
		return fmt.Errorf("mark auth token used: %w", result.Error)
	}

	return nil
}

// InvalidateUnusedTokens marks unused tokens for an email/purpose as used.
func (s *Store) InvalidateUnusedTokens(ctx context.Context, email, purpose string) error {
	result := s.db.WithContext(ctx).
		Model(&authTokenModel{}).
		Where("email = ? AND purpose = ? AND used_at IS NULL",
			strings.ToLower(strings.TrimSpace(email)), purpose).
		Update("used_at", gorm.Expr("NOW()"))
	if result.Error != nil {
		return fmt.Errorf("invalidate auth tokens: %w", result.Error)
	}

	return nil
}

// CreateSession inserts a new session row.
func (s *Store) CreateSession(
	ctx context.Context,
	sessionID, userID, tokenHash string,
	expiresAt time.Time,
	meta auth.SessionMeta,
) error {
	model := sessionModel{
		ID:        sessionID,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		IP:        nullStringPtr(meta.IP),
		UserAgent: nullStringPtr(meta.UserAgent),
	}

	err := s.db.WithContext(ctx).Omit("CreatedAt").Create(&model).Error
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}

	return nil
}

// UpdateSessionRefreshToken rotates the refresh token hash for an existing session.
func (s *Store) UpdateSessionRefreshToken(
	ctx context.Context,
	sessionID, tokenHash string,
	expiresAt time.Time,
) error {
	result := s.db.WithContext(ctx).
		Model(&sessionModel{}).
		Where("id = ?", sessionID).
		Updates(map[string]any{
			"token_hash": tokenHash,
			"expires_at": expiresAt,
		})
	if result.Error != nil {
		return fmt.Errorf("update session refresh token: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return auth.ErrSessionNotFound
	}

	return nil
}

// GetSessionByTokenHash loads a session by hashed token.
func (s *Store) GetSessionByTokenHash(
	ctx context.Context,
	tokenHash string,
) (*auth.SessionRecord, error) {
	var model sessionModel

	err := s.db.WithContext(ctx).
		Where("token_hash = ?", tokenHash).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("session not found: %w", err)
		}

		return nil, fmt.Errorf("select session: %w", err)
	}

	return sessionFromModel(model), nil
}

// DeleteSessionByTokenHash removes a session.
func (s *Store) DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error {
	err := s.db.WithContext(ctx).
		Where("token_hash = ?", tokenHash).
		Delete(&sessionModel{}).Error
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

// DeleteAllSessionsByUserID removes all sessions for a user.
func (s *Store) DeleteAllSessionsByUserID(ctx context.Context, userID string) error {
	err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&sessionModel{}).Error
	if err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}

	return nil
}

// DeleteSessionsExcept removes all sessions except the given session ID.
func (s *Store) DeleteSessionsExcept(ctx context.Context, userID, keepSessionID string) error {
	if keepSessionID == "" {
		return nil
	}

	err := s.db.WithContext(ctx).
		Where("user_id = ? AND id <> ?", userID, keepSessionID).
		Delete(&sessionModel{}).Error
	if err != nil {
		return fmt.Errorf("delete other sessions: %w", err)
	}

	return nil
}

func (s *Store) getUserByEmail(
	ctx context.Context,
	email string,
	withPassword bool,
) (*auth.User, string, error) {
	return s.findUser(ctx, "email", email, withPassword)
}

func (s *Store) getUserByID(
	ctx context.Context,
	userID string,
	withPassword bool,
) (*auth.User, string, error) {
	return s.findUser(ctx, "id", userID, withPassword)
}

func (s *Store) findUser(
	ctx context.Context,
	column, value string,
	withPassword bool,
) (*auth.User, string, error) {
	query := s.db.WithContext(ctx).Model(&userModel{})
	switch column {
	case "email":
		query = query.Where("email = ?", value)
	case "id":
		query = query.Where("id = ?", value)
	default:
		return nil, "", fmt.Errorf("%w: %q", errUnsupportedLookupColumn, column)
	}

	var model userModel
	err := query.First(&model).Error
	if err != nil {
		return nil, "", mapUserLookupError(err, column)
	}

	user := userFromModel(model)
	if withPassword {
		return user, model.PasswordHash, nil
	}

	return user, "", nil
}

func mapUserLookupError(err error, column string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return auth.ErrManagedUserNotFound
	}

	return fmt.Errorf("select user by %s: %w", column, err)
}

// ListSessionsByUserID returns active session rows for a user.
func (s *Store) ListSessionsByUserID(
	ctx context.Context,
	userID string,
) ([]auth.SessionRecord, error) {
	var models []sessionModel

	err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	sessions := make([]auth.SessionRecord, 0, len(models))
	for _, model := range models {
		sessions = append(sessions, *sessionFromModel(model))
	}

	return sessions, nil
}

// GetSessionByID loads a session row by primary key.
func (s *Store) GetSessionByID(ctx context.Context, sessionID string) (*auth.SessionRecord, error) {
	var model sessionModel

	err := s.db.WithContext(ctx).First(&model, "id = ?", sessionID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("session not found: %w", err)
		}

		return nil, fmt.Errorf("select session: %w", err)
	}

	return sessionFromModel(model), nil
}

// DeleteSessionByID removes a session by primary key.
func (s *Store) DeleteSessionByID(ctx context.Context, sessionID string) error {
	err := s.db.WithContext(ctx).
		Delete(&sessionModel{}, "id = ?", sessionID).Error
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

// ListPendingInvites returns unused invite tokens.
func (s *Store) ListPendingInvites(ctx context.Context, email string) ([]auth.Token, error) {
	query := s.db.WithContext(ctx).
		Where("purpose = ? AND used_at IS NULL", auth.TokenPurposeInvite)
	if email != "" {
		query = query.Where("email = ?", email)
	}

	var models []authTokenModel

	err := query.Order("created_at DESC").Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}

	tokens := make([]auth.Token, 0, len(models))
	for _, model := range models {
		tokens = append(tokens, *tokenFromModel(model))
	}

	return tokens, nil
}

// GetAuthTokenByID loads a token by primary key.
func (s *Store) GetAuthTokenByID(ctx context.Context, id string) (*auth.Token, error) {
	var model authTokenModel

	err := s.db.WithContext(ctx).First(&model, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("auth token not found: %w", err)
		}

		return nil, fmt.Errorf("select auth token: %w", err)
	}

	return tokenFromModel(model), nil
}

// UserHasTOTP reports whether a user has confirmed 2FA enabled.
func (s *Store) UserHasTOTP(ctx context.Context, userID string) (bool, error) {
	var model userTOTPModel

	err := s.db.WithContext(ctx).
		Select("enabled_at").
		Where("user_id = ?", userID).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("select user totp: %w", err)
	}

	return model.EnabledAt != nil, nil
}

// GetUserTOTP loads confirmed 2FA state for a user.
func (s *Store) GetUserTOTP(ctx context.Context, userID string) (*auth.TOTPRecord, error) {
	var model userTOTPModel

	err := s.db.WithContext(ctx).
		Where("user_id = ? AND enabled_at IS NOT NULL", userID).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("totp not configured: %w", auth.ErrTOTPNotConfigured)
		}

		return nil, fmt.Errorf("select user totp: %w", err)
	}

	record := &auth.TOTPRecord{
		UserID:          model.UserID,
		SecretEncrypted: model.SecretEncrypted,
		EnabledAt:       *model.EnabledAt,
	}
	err = json.Unmarshal(model.BackupCodesRaw, &record.BackupCodesHash)
	if err != nil {
		return nil, fmt.Errorf("decode backup codes: %w", err)
	}

	return record, nil
}

// SavePendingTOTPSecret stores an encrypted secret before enrollment is confirmed.
func (s *Store) SavePendingTOTPSecret(ctx context.Context, userID, secretEncrypted string) error {
	model := userTOTPModel{
		UserID:          userID,
		SecretEncrypted: secretEncrypted,
		EnabledAt:       nil,
		BackupCodesRaw:  datatypes.JSON("[]"),
	}

	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"secret_encrypted":  secretEncrypted,
			"enabled_at":        nil,
			"backup_codes_hash": datatypes.JSON("[]"),
		}),
	}).Create(&model).Error
	if err != nil {
		return fmt.Errorf("save pending totp secret: %w", err)
	}

	return nil
}

// GetPendingTOTPSecret loads the encrypted secret for pending enrollment.
func (s *Store) GetPendingTOTPSecret(ctx context.Context, userID string) (string, error) {
	var model userTOTPModel

	err := s.db.WithContext(ctx).
		Select("secret_encrypted").
		Where("user_id = ? AND enabled_at IS NULL", userID).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}

	if err != nil {
		return "", fmt.Errorf("select pending totp secret: %w", err)
	}

	return model.SecretEncrypted, nil
}

// EnableUserTOTP marks 2FA enabled for a user.
func (s *Store) EnableUserTOTP(
	ctx context.Context,
	userID, secretEncrypted string,
	backupHashes []string,
) error {
	raw, err := json.Marshal(backupHashes)
	if err != nil {
		return fmt.Errorf("encode backup codes: %w", err)
	}

	model := userTOTPModel{
		UserID:          userID,
		SecretEncrypted: secretEncrypted,
		EnabledAt:       new(time.Now().UTC()),
		BackupCodesRaw:  datatypes.JSON(raw),
	}

	err = s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"secret_encrypted":  secretEncrypted,
			"enabled_at":        gorm.Expr("NOW()"),
			"backup_codes_hash": datatypes.JSON(raw),
		}),
	}).Create(&model).Error
	if err != nil {
		return fmt.Errorf("enable user totp: %w", err)
	}

	return nil
}

// DeleteUserTOTP removes 2FA state for a user.
func (s *Store) DeleteUserTOTP(ctx context.Context, userID string) error {
	err := s.db.WithContext(ctx).
		Delete(&userTOTPModel{}, "user_id = ?", userID).Error
	if err != nil {
		return fmt.Errorf("delete user totp: %w", err)
	}

	return nil
}

// ConsumeBackupCode clears a used backup code hash.
func (s *Store) ConsumeBackupCode(ctx context.Context, userID string, index int) error {
	record, err := s.GetUserTOTP(ctx, userID)
	if err != nil {
		return fmt.Errorf("load totp: %w", err)
	}
	if record == nil || index < 0 || index >= len(record.BackupCodesHash) {
		return auth.ErrInvalidBackupCodeIndex
	}

	record.BackupCodesHash[index] = ""
	raw, err := json.Marshal(record.BackupCodesHash)
	if err != nil {
		return fmt.Errorf("encode backup codes: %w", err)
	}

	result := s.db.WithContext(ctx).
		Model(&userTOTPModel{}).
		Where("user_id = ?", userID).
		Update("backup_codes_hash", datatypes.JSON(raw))
	if result.Error != nil {
		return fmt.Errorf("consume backup code: %w", result.Error)
	}

	return nil
}

// GetPlaybackPreferences loads ordered audio language codes for a user.
func (s *Store) GetPlaybackPreferences(ctx context.Context, userID string) (auth.PlaybackPreferences, error) {
	var model userModel
	err := s.db.WithContext(ctx).Select("audio_language_prefs").Where("id = ?", userID).First(&model).Error
	if err != nil {
		return auth.PlaybackPreferences{}, fmt.Errorf("get playback preferences: %w", err)
	}

	return playbackPrefsFromJSON(model.AudioLanguagePrefs), nil
}

// SavePlaybackPreferences stores ordered audio language codes for a user.
func (s *Store) SavePlaybackPreferences(
	ctx context.Context,
	userID string,
	prefs auth.PlaybackPreferences,
) error {
	raw, err := json.Marshal(prefs.AudioLanguages)
	if err != nil {
		return fmt.Errorf("encode playback preferences: %w", err)
	}

	result := s.db.WithContext(ctx).
		Model(&userModel{}).
		Where("id = ?", userID).
		Updates(map[string]any{
			"audio_language_prefs": datatypes.JSON(raw),
			columnUpdatedAt:        time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("save playback preferences: %w", result.Error)
	}

	return nil
}

func playbackPrefsFromJSON(raw datatypes.JSON) auth.PlaybackPreferences {
	if len(raw) == 0 {
		return auth.PlaybackPreferences{AudioLanguages: []string{}}
	}

	var langs []string
	err := json.Unmarshal(raw, &langs)
	if err != nil {
		return auth.PlaybackPreferences{AudioLanguages: []string{}}
	}

	return auth.PlaybackPreferences{AudioLanguages: langs}
}

func userFromModel(model userModel) *auth.User {
	return &auth.User{
		ID:                 model.ID,
		Email:              model.Email,
		Role:               model.Role,
		Enabled:            model.Enabled,
		EmailVerifiedAt:    model.EmailVerifiedAt,
		MustChangePassword: model.MustChangePassword,
	}
}

func sessionFromModel(model sessionModel) *auth.SessionRecord {
	return &auth.SessionRecord{
		ID:        model.ID,
		UserID:    model.UserID,
		TokenHash: model.TokenHash,
		ExpiresAt: model.ExpiresAt,
		IP:        stringOrEmpty(model.IP),
		UserAgent: stringOrEmpty(model.UserAgent),
		CreatedAt: model.CreatedAt,
	}
}

func tokenFromModel(model authTokenModel) *auth.Token {
	token := &auth.Token{
		ID:        model.ID,
		Email:     model.Email,
		Purpose:   model.Purpose,
		TokenHash: model.TokenHash,
		ExpiresAt: model.ExpiresAt,
		UsedAt:    model.UsedAt,
		CreatedAt: model.CreatedAt,
	}
	if model.UserID != nil {
		token.UserID = *model.UserID
	}
	if model.Role != nil {
		token.Role = *model.Role
	}
	if model.InvitedBy != nil {
		token.InvitedBy = *model.InvitedBy
	}

	return token
}

func authSettingsKey() string {
	return "auth_policy"
}

func nullStringPtr(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}
