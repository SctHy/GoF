package applicationexposure

import (
	domainexposure "GCFeed/internal/domain/exposure"
	"context"
	"errors"
)

var ErrSaveExposureFailed = errors.New("failed to save exposure")

type Service struct {
	repo     domainexposure.Repository
	interest InterestInvalidator
}

type InterestInvalidator interface {
	InvalidateInterest(context.Context, int64) error
}

func WithInterestInvalidator(invalidator InterestInvalidator) Option {
	return func(s *Service) { s.interest = invalidator }
}

type RecordViewEventResult struct {
	Event    *domainexposure.ViewEvent
	Exposure *domainexposure.Exposure
}

type Option func(*Service)

func New(repo domainexposure.Repository, options ...Option) *Service {
	service := &Service{repo: repo}
	for _, option := range options {
		option(service)
	}
	return service
}

// RecordViewEvent 写入观看行为，并在 exposed 事件时同步维护曝光聚合索引。
func (s *Service) RecordViewEvent(ctx context.Context, userID int64, videoID int64, scene string, requestID string, eventType string, watchMs int, completed bool) (*RecordViewEventResult, error) {
	event, err := domainexposure.NewViewEvent(userID, videoID, scene, requestID, eventType, watchMs, completed)
	if err != nil {
		return nil, err
	}

	savedEvent, exposure, err := s.repo.SaveViewEvent(ctx, event)
	if err != nil {
		if errors.Is(err, domainexposure.ErrVideoNotFound) {
			return nil, err
		}
		return nil, ErrSaveExposureFailed
	}
	// 观看事实已提交，画像缓存失效失败由短 TTL 兜底，不能让客户端重复创建事实。
	if s.interest != nil {
		_ = s.interest.InvalidateInterest(ctx, userID)
	}

	return &RecordViewEventResult{
		Event:    savedEvent,
		Exposure: exposure,
	}, nil
}
