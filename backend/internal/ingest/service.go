package ingest

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"live-orchestrator/backend/internal/pagination"
)

// ClientChecker is the subset of client.Service this package depends on. The
// interface is declared here, in the consumer, not in internal/client: Go
// gives implicit interface satisfaction for free, so there's no need for a
// shared abstractions package.
type ClientChecker interface {
	Exists(ctx context.Context, tx *gorm.DB, id uuid.UUID) (bool, error)
}

// ErrURLRequired is returned when a PATCH body has neither url nor isActive.
var ErrURLRequired = errors.New("at least one of url or isActive must be provided")

type Service struct {
	db      *gorm.DB
	clients ClientChecker
}

func NewService(db *gorm.DB, clients ClientChecker) *Service {
	return &Service{db: db, clients: clients}
}

// ListFilter narrows a listing by client and/or active state.
type ListFilter struct {
	ClientID *uuid.UUID
	IsActive *bool
}

// Create checks the parent client's existence and inserts the ingest inside
// the same transaction: this closes the race between "check" and "write",
// and gives a clean 404 instead of surfacing the FK violation.
func (s *Service) Create(ctx context.Context, clientID uuid.UUID, req CreateRequest) (*Ingest, error) {
	ing, err := New(clientID, req.URL, req.ActiveOrDefault())
	if err != nil {
		return nil, err
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ok, err := s.clients.Exists(ctx, tx, clientID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrClientNotFound
		}

		var count int64
		if err := tx.Model(&Ingest{}).Where("client_id = ? AND url = ?", clientID, ing.URL).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrDuplicateURL
		}
		return tx.Create(ing).Error
	})
	if err != nil {
		return nil, err
	}
	return ing, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*Ingest, error) {
	var ing Ingest
	err := s.db.WithContext(ctx).First(&ing, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ing, nil
}

// List returns ingests matching filter, ordered by (created_at DESC, id
// DESC), paginated via opaque cursor. If filter.ClientID is set and does not
// reference an existing client, ErrClientNotFound is returned.
func (s *Service) List(ctx context.Context, filter ListFilter, page pagination.Request) (pagination.Page[Response], error) {
	if filter.ClientID != nil {
		ok, err := s.clients.Exists(ctx, s.db.WithContext(ctx), *filter.ClientID)
		if err != nil {
			return pagination.Page[Response]{}, err
		}
		if !ok {
			return pagination.Page[Response]{}, ErrClientNotFound
		}
	}

	q := s.db.WithContext(ctx).Model(&Ingest{}).Order("created_at DESC, id DESC").Limit(page.Limit + 1)
	if filter.ClientID != nil {
		q = q.Where("client_id = ?", *filter.ClientID)
	}
	if filter.IsActive != nil {
		q = q.Where("is_active = ?", *filter.IsActive)
	}
	if page.Cursor != nil {
		q = q.Where("(created_at, id) < (?, ?)", page.Cursor.CreatedAt, page.Cursor.ID)
	}

	var rows []*Ingest
	if err := q.Find(&rows).Error; err != nil {
		return pagination.Page[Response]{}, err
	}

	return pagination.NewPage(ToResponses(rows), page.Limit, func(r Response) pagination.Cursor {
		return pagination.Cursor{CreatedAt: r.CreatedAt, ID: r.ID}
	}), nil
}

// Update applies a partial update. Changing URL re-derives Protocol so the
// two never disagree. At least one of url/isActive must be present.
func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateRequest) (*Ingest, error) {
	if req.URL == nil && req.IsActive == nil {
		return nil, ErrURLRequired
	}

	var updated Ingest
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ing Ingest
		if err := tx.First(&ing, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}

		if req.URL != nil {
			if err := ing.ChangeURL(*req.URL); err != nil {
				return err
			}
			var count int64
			if err := tx.Model(&Ingest{}).
				Where("client_id = ? AND url = ? AND id <> ?", ing.ClientID, ing.URL, id).
				Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrDuplicateURL
			}
		}
		if req.IsActive != nil {
			ing.IsActive = *req.IsActive
		}

		if err := tx.Save(&ing).Error; err != nil {
			return err
		}
		updated = ing
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

// Delete is a hard delete: to deactivate without removing, PATCH isActive=false.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	res := s.db.WithContext(ctx).Delete(&Ingest{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
