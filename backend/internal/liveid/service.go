package liveid

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"live-orchestrator/backend/internal/pagination"
)

// ClientChecker and PlatformChecker are declared here, in the consumer
// package: client.Service and streamplatform.Service satisfy them without
// knowing they exist.
type ClientChecker interface {
	Exists(ctx context.Context, tx *gorm.DB, id uuid.UUID) (bool, error)
}

type PlatformChecker interface {
	Exists(ctx context.Context, tx *gorm.DB, id uuid.UUID) (bool, error)
}

type Service struct {
	db        *gorm.DB
	clients   ClientChecker
	platforms PlatformChecker
}

func NewService(db *gorm.DB, clients ClientChecker, platforms PlatformChecker) *Service {
	return &Service{db: db, clients: clients, platforms: platforms}
}

// ListFilter narrows a listing by client, platform and/or active state.
type ListFilter struct {
	ClientID   *uuid.UUID
	PlatformID *uuid.UUID
	IsActive   *bool
}

// Create checks both the client and the platform inside the same transaction
// as the insert (same TOCTOU reasoning as ingest.Service.Create).
func (s *Service) Create(ctx context.Context, clientID uuid.UUID, req CreateRequest) (*ClientLiveID, error) {
	l, err := New(clientID, req.PlatformID, req.LiveID, req.ActiveOrDefault())
	if err != nil {
		return nil, err
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		clientOK, err := s.clients.Exists(ctx, tx, clientID)
		if err != nil {
			return err
		}
		if !clientOK {
			return ErrClientNotFound
		}
		platformOK, err := s.platforms.Exists(ctx, tx, req.PlatformID)
		if err != nil {
			return err
		}
		if !platformOK {
			return ErrPlatformNotFound
		}

		var count int64
		if err := tx.Model(&ClientLiveID{}).
			Where("client_id = ? AND platform_id = ? AND live_id = ?", clientID, req.PlatformID, l.LiveID).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrDuplicateLiveID
		}
		return tx.Create(l).Error
	})
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*ClientLiveID, error) {
	var l ClientLiveID
	err := s.db.WithContext(ctx).First(&l, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

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

	q := s.db.WithContext(ctx).Model(&ClientLiveID{}).Order("created_at DESC, id DESC").Limit(page.Limit + 1)
	if filter.ClientID != nil {
		q = q.Where("client_id = ?", *filter.ClientID)
	}
	if filter.PlatformID != nil {
		q = q.Where("platform_id = ?", *filter.PlatformID)
	}
	if filter.IsActive != nil {
		q = q.Where("is_active = ?", *filter.IsActive)
	}
	if page.Cursor != nil {
		q = q.Where("(created_at, id) < (?, ?)", page.Cursor.CreatedAt, page.Cursor.ID)
	}

	var rows []*ClientLiveID
	if err := q.Find(&rows).Error; err != nil {
		return pagination.Page[Response]{}, err
	}

	return pagination.NewPage(ToResponses(rows), page.Limit, func(r Response) pagination.Cursor {
		return pagination.Cursor{CreatedAt: r.CreatedAt, ID: r.ID}
	}), nil
}

// Update applies a partial update. Only LiveID and IsActive are editable;
// PlatformID/ClientID reassignment is not offered here by design.
func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateRequest) (*ClientLiveID, error) {
	var updated ClientLiveID
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var l ClientLiveID
		if err := tx.First(&l, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}

		if req.LiveID != nil {
			liveID, err := normalizeLiveID(*req.LiveID)
			if err != nil {
				return err
			}
			var count int64
			if err := tx.Model(&ClientLiveID{}).
				Where("client_id = ? AND platform_id = ? AND live_id = ? AND id <> ?", l.ClientID, l.PlatformID, liveID, id).
				Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrDuplicateLiveID
			}
			l.LiveID = liveID
		}
		if req.IsActive != nil {
			l.IsActive = *req.IsActive
		}

		if err := tx.Save(&l).Error; err != nil {
			return err
		}
		updated = l
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

// Delete is a hard delete: to deactivate without removing, PATCH isActive=false.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	res := s.db.WithContext(ctx).Delete(&ClientLiveID{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
