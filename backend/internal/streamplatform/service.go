package streamplatform

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"live-orchestrator/backend/internal/pagination"
)

// postgresForeignKeyViolation is the SQLSTATE Postgres returns when a DELETE
// is blocked by a referencing row (client_live_ids.platform_id has no ON
// DELETE clause, so the FK itself — not application code — blocks removal
// of a platform still in use).
const postgresForeignKeyViolation = "23503"

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (*Platform, error) {
	p, err := New(req.Slug, req.DisplayName)
	if err != nil {
		return nil, err
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&Platform{}).Where("slug = ?", p.Slug).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrDuplicateSlug
		}
		return tx.Create(p).Error
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*Platform, error) {
	var p Platform
	err := s.db.WithContext(ctx).First(&p, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Service) List(ctx context.Context, page pagination.Request) (pagination.Page[Response], error) {
	q := s.db.WithContext(ctx).Model(&Platform{}).Order("created_at DESC, id DESC").Limit(page.Limit + 1)
	if page.Cursor != nil {
		q = q.Where("(created_at, id) < (?, ?)", page.Cursor.CreatedAt, page.Cursor.ID)
	}

	var rows []*Platform
	if err := q.Find(&rows).Error; err != nil {
		return pagination.Page[Response]{}, err
	}

	return pagination.NewPage(ToResponses(rows), page.Limit, func(r Response) pagination.Cursor {
		return pagination.Cursor{CreatedAt: r.CreatedAt, ID: r.ID}
	}), nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateRequest) (*Platform, error) {
	var updated Platform
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p Platform
		if err := tx.First(&p, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}

		if req.Slug != nil {
			slug := normalizeSlug(*req.Slug)
			if slug == "" || len(slug) > maxSlugLen {
				return ErrInvalidSlug
			}
			var count int64
			if err := tx.Model(&Platform{}).Where("slug = ? AND id <> ?", slug, id).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrDuplicateSlug
			}
			p.Slug = slug
		}
		if req.DisplayName != nil {
			name := strings.TrimSpace(*req.DisplayName)
			if name == "" || len(name) > maxNameLen {
				return ErrInvalidName
			}
			p.DisplayName = name
		}

		if err := tx.Save(&p).Error; err != nil {
			return err
		}
		updated = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

// Delete hard-deletes a platform. If any client_live_ids row still
// references it, the FK (declared without ON DELETE) blocks the delete at
// the database level; that violation is translated to ErrPlatformInUse.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	res := s.db.WithContext(ctx).Delete(&Platform{}, "id = ?", id)
	if res.Error != nil {
		var pgErr *pgconn.PgError
		if errors.As(res.Error, &pgErr) && pgErr.Code == postgresForeignKeyViolation {
			return ErrPlatformInUse
		}
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Exists satisfies the liveid package's PlatformChecker interface.
func (s *Service) Exists(ctx context.Context, tx *gorm.DB, id uuid.UUID) (bool, error) {
	var count int64
	if err := tx.WithContext(ctx).Model(&Platform{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
