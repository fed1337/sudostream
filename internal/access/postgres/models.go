package postgres

import "sudoStream/internal/access"

type libraryModel struct {
	ID    string             `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	Slug  string             `gorm:"column:slug"`
	Name  string             `gorm:"column:name"`
	Type  access.LibraryType `gorm:"column:type"`
	Roots []libraryRootModel `gorm:"foreignKey:LibraryID"`
}

func (libraryModel) TableName() string {
	return "libraries"
}

type libraryRootModel struct {
	LibraryID string `gorm:"column:library_id;primaryKey;type:uuid"`
	RelPath   string `gorm:"column:rel_path;primaryKey"`
}

func (libraryRootModel) TableName() string {
	return "library_roots"
}

type libraryGrantModel struct {
	UserID    string `gorm:"column:user_id;primaryKey;type:uuid"`
	LibraryID string `gorm:"column:library_id;primaryKey;type:uuid"`
	CanCreate bool   `gorm:"column:can_create"`
	CanRead   bool   `gorm:"column:can_read"`
	CanUpdate bool   `gorm:"column:can_update"`
	CanDelete bool   `gorm:"column:can_delete"`
}

func (libraryGrantModel) TableName() string {
	return "library_grants"
}

func libraryFromModel(model libraryModel) access.Library {
	roots := make([]string, 0, len(model.Roots))
	for _, root := range model.Roots {
		roots = append(roots, root.RelPath)
	}

	relPath := ""
	if len(roots) > 0 {
		relPath = roots[0]
	}

	return access.Library{
		ID:      model.ID,
		Slug:    model.Slug,
		RelPath: relPath,
		Name:    model.Name,
		Type:    model.Type,
		Roots:   roots,
	}
}
