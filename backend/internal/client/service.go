package client

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"live-orchestrator/backend/internal/pagination"
)

// Service holds the business rules for clients: uniqueness of name, partial
// update semantics, and soft delete.
type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// Create inserts a client. Name uniqueness is enforced here, not by a DB
// constraint: this is a service-layer check + insert, run inside a
// transaction to close the window between the check and the write.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Client, error) {
	c, err := New(req.Name, req.Email)
	if err != nil {
		return nil, err
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&Client{}).Where("name = ?", c.Name).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrDuplicateName
		}
		return tx.Create(c).Error
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*Client, error) {
	var c Client
	err := s.db.WithContext(ctx).First(&c, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// List returns non-deleted clients ordered by (created_at DESC, id DESC),
// paginated via an opaque keyset cursor.
func (s *Service) List(ctx context.Context, page pagination.Request) (pagination.Page[Response], error) {
	q := s.db.WithContext(ctx).Model(&Client{}).Order("created_at DESC, id DESC").Limit(page.Limit + 1)
	if page.Cursor != nil {
		q = q.Where("(created_at, id) < (?, ?)", page.Cursor.CreatedAt, page.Cursor.ID)
	}

	var rows []*Client
	if err := q.Find(&rows).Error; err != nil {
		return pagination.Page[Response]{}, err
	}

	return pagination.NewPage(ToResponses(rows), page.Limit, func(r Response) pagination.Cursor {
		return pagination.Cursor{CreatedAt: r.CreatedAt, ID: r.ID}
	}), nil
}

// Update applies a partial update. Name, if present, must not be blank after
// trimming and must remain unique. Email, if the key was present in the
// body, is either replaced (non-null) or cleared (explicit null).
func (s *Service) Update(ctx context.Context, id uuid.UUID, fields UpdateFields) (*Client, error) {
	var name *string
	if fields.Name != nil {
		trimmed := strings.TrimSpace(*fields.Name)
		if trimmed == "" || len(trimmed) > maxNameLen {
			return nil, ErrInvalidName
		}
		name = &trimmed
	}

	var email *string
	if fields.EmailSet {
		normalized, err := normalizeEmail(fields.EmailValue)
		if err != nil {
			return nil, err
		}
		email = normalized
	}

	var updated Client
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var c Client
		if err := tx.First(&c, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}

		if name != nil {
			var count int64
			if err := tx.Model(&Client{}).Where("name = ? AND id <> ?", *name, id).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrDuplicateName
			}
			c.Name = *name
		}
		if fields.EmailSet {
			c.Email = email
		}

		if err := tx.Save(&c).Error; err != nil {
			return err
		}
		updated = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

// Delete soft-deletes a client: ingests and live ids it owns are left
// untouched and remain reachable by their own ids.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	res := s.db.WithContext(ctx).Delete(&Client{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Exists reports whether a (non-deleted) client with this id exists. Used by
// the ingest and liveid services to check the parent client inside their own
// write transaction (TOCTOU-safe existence check).
func (s *Service) Exists(ctx context.Context, tx *gorm.DB, id uuid.UUID) (bool, error) {
	var count int64
	if err := tx.WithContext(ctx).Model(&Client{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
