package postgres

import (
	"context"
	"errors"
	"fmt"
	"sudoStream/internal/auth"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type externalIdentityModel struct {
	ID        string    `gorm:"column:id;primaryKey;type:uuid"`
	UserID    string    `gorm:"column:user_id;type:uuid"`
	Issuer    string    `gorm:"column:issuer"`
	Subject   string    `gorm:"column:subject"`
	IDToken   *string   `gorm:"column:id_token"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (externalIdentityModel) TableName() string {
	return "external_identities"
}

// GetExternalIdentityBySubject loads a mapping by IdP issuer + subject.
func (s *Store) GetExternalIdentityBySubject(
	ctx context.Context,
	issuer, subject string,
) (*auth.ExternalIdentity, error) {
	var model externalIdentityModel
	err := s.db.WithContext(ctx).
		Where("issuer = ? AND subject = ?", issuer, subject).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, auth.ErrExternalIdentityNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get external identity: %w", err)
	}

	return toExternalIdentity(model), nil
}

// GetExternalIdentityByUserID loads the most recently updated mapping for a user.
func (s *Store) GetExternalIdentityByUserID(
	ctx context.Context,
	userID string,
) (*auth.ExternalIdentity, error) {
	var model externalIdentityModel
	err := s.db.WithContext(ctx).
		Where(columnUserID+" = ?", userID).
		Order(columnUpdatedAt + " DESC").
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, auth.ErrExternalIdentityNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get external identity by user: %w", err)
	}

	return toExternalIdentity(model), nil
}

// UpsertExternalIdentity inserts or updates the (issuer, subject) → user mapping.
func (s *Store) UpsertExternalIdentity(ctx context.Context, identity auth.ExternalIdentity) error {
	now := time.Now().UTC()
	model := externalIdentityModel{
		UserID:    identity.UserID,
		Issuer:    identity.Issuer,
		Subject:   identity.Subject,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if identity.IDToken != "" {
		token := identity.IDToken
		model.IDToken = &token
	}

	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "issuer"}, {Name: "subject"}},
		DoUpdates: clause.Assignments(map[string]any{
			columnUserID:    identity.UserID,
			"id_token":      model.IDToken,
			columnUpdatedAt: now,
		}),
	}).Omit("ID").Create(&model).Error
	if err != nil {
		return fmt.Errorf("upsert external identity: %w", err)
	}

	return nil
}

// UpdateExternalIdentityIDToken stores the latest id_token for RP-initiated logout.
func (s *Store) UpdateExternalIdentityIDToken(
	ctx context.Context,
	issuer, subject, idToken string,
) error {
	result := s.db.WithContext(ctx).
		Model(&externalIdentityModel{}).
		Where("issuer = ? AND subject = ?", issuer, subject).
		Updates(map[string]any{
			"id_token":   idToken,
			"updated_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("update external identity id_token: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return auth.ErrExternalIdentityNotFound
	}

	return nil
}

func toExternalIdentity(model externalIdentityModel) *auth.ExternalIdentity {
	idToken := ""
	if model.IDToken != nil {
		idToken = *model.IDToken
	}

	return &auth.ExternalIdentity{
		ID:        model.ID,
		UserID:    model.UserID,
		Issuer:    model.Issuer,
		Subject:   model.Subject,
		IDToken:   idToken,
		CreatedAt: model.CreatedAt,
		UpdatedAt: model.UpdatedAt,
	}
}
