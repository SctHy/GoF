package applicationinteraction

import (
	domaininteraction "GCFeed/internal/domain/interaction"
	"context"
	"errors"
	"time"
)

type ActionEffectsProjector interface {
	ProjectActionEffects(context.Context, *ActionChangedEvent) error
}
type ActionNotificationProjector interface {
	ProjectActionNotification(context.Context, *ActionChangedEvent) error
}
type IdempotentHotScoreRecorder interface {
	AddHotScoreOnce(context.Context, string, int64, int, time.Time) error
}
type ActionEffects struct {
	notifications ActionNotificationProjector
	hot           IdempotentHotScoreRecorder
}

func NewActionEffects(notifications ActionNotificationProjector, hot IdempotentHotScoreRecorder) *ActionEffects {
	return &ActionEffects{notifications: notifications, hot: hot}
}

// ProjectActionEffects 按顺序重试通知与热榜；通知按用户/视频、热榜按事件 ID 去重。
func (p *ActionEffects) ProjectActionEffects(ctx context.Context, event *ActionChangedEvent) error {
	if err := validateActionEvent(event); err != nil {
		return err
	}
	weight := 0
	switch event.ActionType {
	case domaininteraction.ActionTypeLike:
		weight = hotScoreLikeWeight
	case domaininteraction.ActionTypeFavorite:
		weight = hotScoreFavoriteWeight
	default:
		return domaininteraction.ErrInvalidActionType
	}
	if event.Delta == 0 {
		return nil
	}
	if event.ActionType == domaininteraction.ActionTypeLike && event.Delta > 0 {
		if p.notifications == nil {
			return errors.New("action notification projector is required")
		}
		if err := p.notifications.ProjectActionNotification(ctx, event); err != nil {
			return err
		}
	}
	if p.hot == nil {
		return errors.New("idempotent hot score recorder is required")
	}
	return p.hot.AddHotScoreOnce(ctx, event.EventID, event.VideoID, event.Delta*weight, event.OccurredAt)
}

func validateActionEvent(event *ActionChangedEvent) error {
	if event == nil || event.EffectsVersion != 1 || event.Version == 0 || event.EventID == "" || event.VideoID <= 0 || event.UserID <= 0 || event.OccurredAt.IsZero() || event.Delta < -1 || event.Delta > 1 || (event.Delta > 0 && !event.Active) || (event.Delta < 0 && event.Active) {
		return errors.New("invalid action event")
	}
	if event.ActionType != domaininteraction.ActionTypeLike && event.ActionType != domaininteraction.ActionTypeFavorite {
		return domaininteraction.ErrInvalidActionType
	}
	return nil
}
