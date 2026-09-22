package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sudoStream/internal/observability"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

var (
	// ErrTwoFactorRequired is returned when login must continue with 2FA.
	ErrTwoFactorRequired = errors.New("two factor required")
	// ErrTwoFactorSetupRequired is returned when enrollment is required before login.
	ErrTwoFactorSetupRequired = errors.New("two factor setup required")
	// ErrTwoFactorNotConfigured is returned when 2FA is not enabled for the user.
	ErrTwoFactorNotConfigured = errors.New("two factor not configured")
	// ErrTOTPNotConfigured is returned when no confirmed TOTP record exists.
	ErrTOTPNotConfigured = errors.New("totp not configured")
	// ErrInvalidTwoFactorCode is returned when a TOTP or backup code is invalid.
	ErrInvalidTwoFactorCode = errors.New("invalid two factor code")
	// ErrTwoFactorAlreadyEnabled is returned when enrollment is attempted twice.
	ErrTwoFactorAlreadyEnabled = errors.New("two factor already enabled")
	// ErrInvalidBackupCodeIndex is returned when a backup code index is out of range.
	ErrInvalidBackupCodeIndex = errors.New("invalid backup code index")
)

// StartTwoFactorSetup generates a TOTP secret for enrollment.
func (s *Service) StartTwoFactorSetup(
	ctx context.Context,
	userID, email string,
) (TwoFactorSetup, error) {
	enabled, err := s.store.UserHasTOTP(ctx, userID)
	if err != nil {
		return TwoFactorSetup{}, fmt.Errorf("check totp: %w", err)
	}
	if enabled {
		return TwoFactorSetup{}, ErrTwoFactorAlreadyEnabled
	}

	key, err := totp.Generate(
		totp.GenerateOpts{ //nolint:exhaustruct // library defaults are sufficient
			Issuer:      "sudoStream",
			AccountName: email,
			Period:      totpPeriodSeconds,
			Digits:      otp.DigitsSix,
			Algorithm:   otp.AlgorithmSHA1,
		},
	)
	if err != nil {
		return TwoFactorSetup{}, fmt.Errorf("generate totp key: %w", err)
	}

	encrypted, err := encryptTOTPSecret(s.config.TOTPEncryptionKey, key.Secret())
	if err != nil {
		return TwoFactorSetup{}, fmt.Errorf("encrypt pending secret: %w", err)
	}

	err = s.store.SavePendingTOTPSecret(ctx, userID, encrypted)
	if err != nil {
		return TwoFactorSetup{}, fmt.Errorf("store pending totp secret: %w", err)
	}

	return TwoFactorSetup{
		Secret:     key.Secret(),
		OtpauthURL: key.URL(),
	}, nil
}

// ConfirmTwoFactorSetup verifies the first code and enables 2FA.
func (s *Service) ConfirmTwoFactorSetup(
	ctx context.Context,
	userID, code string,
) (TwoFactorConfirmResult, error) {
	encrypted, err := s.store.GetPendingTOTPSecret(ctx, userID)
	if err != nil {
		return TwoFactorConfirmResult{}, fmt.Errorf("load pending secret: %w", err)
	}
	if encrypted == "" {
		return TwoFactorConfirmResult{}, ErrTwoFactorNotConfigured
	}

	secret, err := decryptTOTPSecret(s.config.TOTPEncryptionKey, encrypted)
	if err != nil {
		return TwoFactorConfirmResult{}, fmt.Errorf("decrypt pending secret: %w", err)
	}

	if !s.validateTOTPCode(secret, code, s.totpLeewaySeconds()) {
		return TwoFactorConfirmResult{}, ErrInvalidTwoFactorCode
	}

	backupCodes, backupHashes, err := generateBackupCodes()
	if err != nil {
		return TwoFactorConfirmResult{}, err
	}

	err = s.store.EnableUserTOTP(ctx, userID, encrypted, backupHashes)
	if err != nil {
		return TwoFactorConfirmResult{}, fmt.Errorf("enable totp: %w", err)
	}

	return TwoFactorConfirmResult{BackupCodes: backupCodes}, nil
}

// DisableTwoFactor removes 2FA after password and code verification.
func (s *Service) DisableTwoFactor(
	ctx context.Context,
	userID, password, code string,
) error {
	user, hash, err := s.store.GetUserWithPasswordByID(ctx, userID)
	if err != nil {
		return ErrInvalidCredentials
	}

	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return ErrInvalidCredentials
	}

	record, err := s.store.GetUserTOTP(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrTOTPNotConfigured) {
			return ErrTwoFactorNotConfigured
		}

		return fmt.Errorf("load totp: %w", err)
	}

	valid, err := s.verifyDisableTwoFactor(ctx, userID, record, code)
	if err != nil {
		return err
	}
	if !valid {
		return ErrInvalidTwoFactorCode
	}

	err = s.store.DeleteUserTOTP(ctx, userID)
	if err != nil {
		return fmt.Errorf("delete totp: %w", err)
	}

	_ = user

	return nil
}

func (s *Service) verifyDisableTwoFactor(
	ctx context.Context,
	userID string,
	record *TOTPRecord,
	code string,
) (bool, error) {
	if record == nil {
		return false, ErrTwoFactorNotConfigured
	}

	secret, err := decryptTOTPSecret(s.config.TOTPEncryptionKey, record.SecretEncrypted)
	if err != nil {
		return false, fmt.Errorf("decrypt secret: %w", err)
	}

	if s.validateTOTPCode(secret, code, s.totpLeewaySeconds()) {
		return true, nil
	}

	return s.validateBackupCode(ctx, userID, record, code)
}

// UserHas2FA reports whether the user has confirmed TOTP enabled.
func (s *Service) UserHas2FA(ctx context.Context, userID string) (bool, error) {
	has, err := s.store.UserHasTOTP(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("check totp: %w", err)
	}

	return has, nil
}

// AdminResetTwoFactor clears 2FA for a user (admin recovery).
func (s *Service) AdminResetTwoFactor(ctx context.Context, userID string) error {
	err := s.store.DeleteUserTOTP(ctx, userID)
	if err != nil {
		return fmt.Errorf("delete totp: %w", err)
	}

	err = s.store.DeleteAllSessionsByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("revoke sessions: %w", err)
	}

	return nil
}

// VerifyLoginTwoFactor completes login after password verification.
func (s *Service) VerifyLoginTwoFactor(
	ctx context.Context,
	rawPendingToken, code string,
	meta SessionMeta,
) (PublicUser, string, string, error) {
	token, err := s.validateToken(ctx, rawPendingToken, TokenPurposeLogin2FA)
	if err != nil {
		return PublicUser{}, "", "", err
	}

	user, err := s.store.GetUserByID(ctx, token.UserID)
	if err != nil {
		return PublicUser{}, "", "", ErrInvalidToken
	}
	if !user.Enabled {
		return PublicUser{}, "", "", ErrUserDisabled
	}

	record, err := s.store.GetUserTOTP(ctx, user.ID)
	if err != nil {
		return PublicUser{}, "", "", fmt.Errorf("load totp: %w", err)
	}

	verified, err := s.verifyTwoFactorCode(
		ctx,
		user.ID,
		record,
		code,
		s.totpLeewaySeconds(),
	)
	if err != nil {
		return PublicUser{}, "", "", err
	}
	if !verified {
		return PublicUser{}, "", "", ErrInvalidTwoFactorCode
	}

	err = s.store.MarkAuthTokenUsed(ctx, token.ID)
	if err != nil {
		return PublicUser{}, "", "", fmt.Errorf("consume pending login token: %w", err)
	}

	return s.issueSession(ctx, user, meta)
}

// LoginWithTOTP authenticates credentials and, when configured, completes TOTP in one request.
// Empty codes retain the pending-login response used by the browser flow.
func (s *Service) LoginWithTOTP(
	ctx context.Context,
	email, password, code string,
	meta SessionMeta,
) (LoginOutcome, error) {
	user, hash, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		observability.RecordAuthLoginAttempt("invalid_credentials")

		return LoginOutcome{}, ErrInvalidCredentials
	}

	inviteErr := s.rejectIfInvitePending(ctx, user)
	if inviteErr != nil {
		return LoginOutcome{}, inviteErr
	}

	if !user.Enabled {
		observability.RecordAuthLoginAttempt("disabled")

		return LoginOutcome{}, ErrUserDisabled
	}

	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		observability.RecordAuthLoginAttempt("invalid_credentials")

		return LoginOutcome{}, ErrInvalidCredentials
	}

	hasTOTP, err := s.store.UserHasTOTP(ctx, user.ID)
	if err != nil {
		return LoginOutcome{}, fmt.Errorf("check totp: %w", err)
	}
	if !hasTOTP {
		settings := s.loadSettings(ctx)
		if settings.TwoFactorRequired {
			return s.pendingTwoFactorOutcome(ctx, user, false, true)
		}

		return s.issueLoginOutcome(ctx, user, meta)
	}

	return s.loginWithProvidedTOTP(ctx, user, code, meta)
}

func (s *Service) loginWithProvidedTOTP(
	ctx context.Context,
	user *User,
	code string,
	meta SessionMeta,
) (LoginOutcome, error) {
	if strings.TrimSpace(code) == "" {
		return s.pendingTwoFactorOutcome(ctx, user, true, false)
	}

	record, err := s.store.GetUserTOTP(ctx, user.ID)
	if err != nil {
		return LoginOutcome{}, fmt.Errorf("load totp: %w", err)
	}
	verified, err := s.verifyTwoFactorCode(
		ctx,
		user.ID,
		record,
		code,
		s.totpLeewaySeconds(),
	)
	if err != nil {
		return LoginOutcome{}, err
	}
	if !verified {
		observability.RecordAuthLoginAttempt("invalid_totp")

		return LoginOutcome{}, ErrInvalidTwoFactorCode
	}

	return s.issueLoginOutcome(ctx, user, meta)
}

// CompletePendingTwoFactorSetup enrolls 2FA during login and creates a session.
func (s *Service) CompletePendingTwoFactorSetup(
	ctx context.Context,
	rawPendingToken, code string,
	meta SessionMeta,
) (PublicUser, string, string, TwoFactorConfirmResult, error) {
	token, err := s.validateToken(ctx, rawPendingToken, TokenPurposeLogin2FA)
	if err != nil {
		return PublicUser{}, "", "", TwoFactorConfirmResult{}, err
	}

	user, err := s.store.GetUserByID(ctx, token.UserID)
	if err != nil {
		return PublicUser{}, "", "", TwoFactorConfirmResult{}, ErrInvalidToken
	}

	result, err := s.ConfirmTwoFactorSetup(ctx, user.ID, code)
	if err != nil {
		return PublicUser{}, "", "", TwoFactorConfirmResult{}, err
	}

	err = s.store.MarkAuthTokenUsed(ctx, token.ID)
	if err != nil {
		return PublicUser{}, "", "", TwoFactorConfirmResult{}, fmt.Errorf(
			"consume pending login token: %w",
			err,
		)
	}

	publicUser, accessToken, refreshToken, err := s.issueSession(ctx, user, meta)
	if err != nil {
		return PublicUser{}, "", "", TwoFactorConfirmResult{}, err
	}

	return publicUser, accessToken, refreshToken, result, nil
}

func (s *Service) validateTOTPCode(secret, code string, leewaySeconds int) bool {
	if leewaySeconds < minTwoFactorLeewaySeconds {
		leewaySeconds = defaultTwoFactorLeeway
	}
	if leewaySeconds > maxTwoFactorLeewaySeconds {
		leewaySeconds = maxTwoFactorLeewaySeconds
	}

	skew := max(uint(leewaySeconds/totpPeriodSeconds), 1)

	valid, err := totp.ValidateCustom(
		code,
		secret,
		time.Now().UTC(),
		totp.ValidateOpts{ //nolint:exhaustruct // library defaults are sufficient
			Period:    totpPeriodSeconds,
			Skew:      skew,
			Digits:    otp.DigitsSix,
			Algorithm: otp.AlgorithmSHA1,
		},
	)

	return err == nil && valid
}

func (s *Service) validateBackupCode(
	ctx context.Context,
	userID string,
	record *TOTPRecord,
	code string,
) (bool, error) {
	normalized := strings.ToUpper(strings.TrimSpace(code))
	for index, hash := range record.BackupCodesHash {
		if hash == "" {
			continue
		}

		err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(normalized))
		if err == nil {
			err = s.store.ConsumeBackupCode(ctx, userID, index)
			if err != nil {
				return false, fmt.Errorf("consume backup code: %w", err)
			}

			return true, nil
		}
	}

	return false, nil
}

func generateBackupCodes() ([]string, []string, error) {
	codes := make([]string, backupCodeCount)
	hashes := make([]string, backupCodeCount)

	for i := range codes {
		code, err := randomBackupCode()
		if err != nil {
			return nil, nil, err
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(code), effectiveBcryptCost())
		if err != nil {
			return nil, nil, fmt.Errorf("hash backup code: %w", err)
		}

		codes[i] = code
		hashes[i] = string(hash)
	}

	return codes, hashes, nil
}

func (s *Service) verifyTwoFactorCode(
	ctx context.Context,
	userID string,
	record *TOTPRecord,
	code string,
	leewaySeconds int,
) (bool, error) {
	if record == nil {
		return false, nil
	}

	secret, err := decryptTOTPSecret(s.config.TOTPEncryptionKey, record.SecretEncrypted)
	if err != nil {
		return false, fmt.Errorf("decrypt secret: %w", err)
	}
	if s.validateTOTPCode(secret, code, leewaySeconds) {
		return true, nil
	}

	if !looksLikeBackupCode(code) {
		return false, nil
	}

	return s.validateBackupCode(ctx, userID, record, code)
}

func looksLikeBackupCode(code string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(code))
	if len(normalized) != backupCodeLength {
		return false
	}

	for i := range len(normalized) {
		if !strings.ContainsRune(backupCodeAlphabet, rune(normalized[i])) {
			return false
		}
	}

	return true
}

func randomBackupCode() (string, error) {
	builder := strings.Builder{}
	for range backupCodeLength {
		index, err := rand.Int(rand.Reader, big.NewInt(int64(len(backupCodeAlphabet))))
		if err != nil {
			return "", fmt.Errorf("random backup code: %w", err)
		}

		builder.WriteByte(backupCodeAlphabet[index.Int64()])
	}

	return builder.String(), nil
}

// UserFromPendingLoginToken resolves the user attached to a pending login token.
func (s *Service) UserFromPendingLoginToken(ctx context.Context, rawToken string) (*User, error) {
	token, err := s.validateToken(ctx, rawToken, TokenPurposeLogin2FA)
	if err != nil {
		return nil, err
	}

	user, err := s.store.GetUserByID(ctx, token.UserID)
	if err != nil {
		return nil, ErrInvalidToken
	}

	return user, nil
}
