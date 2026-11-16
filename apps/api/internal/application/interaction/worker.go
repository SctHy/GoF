package applicationinteraction

import (
	inframetrics "GCFeed/internal/infra/metrics"
	"context"
	"time"
)

type ActionEventConsumer interface {
	ConsumeActionChanged(context.Context, func(context.Context, *ActionChangedEvent) error) error
}

type ActionWorker struct {
	consumer    ActionEventConsumer
	invalidator StatCache
	effects     ActionEffectsProjector
}

func NewActionWorker(consumer ActionEventConsumer, invalidator StatCache, effects ActionEffectsProjector) *ActionWorker {
	return &ActionWorker{consumer: consumer, invalidator: invalidator, effects: effects}
}

func (w *ActionWorker) Start(ctx context.Context) error {
	return w.consumer.ConsumeActionChanged(ctx, w.HandleActionChanged)
}

// 每个投影自行幂等，重投必须重试失败的投影；Worker 不再写互动事实。
func (w *ActionWorker) HandleActionChanged(ctx context.Context, event *ActionChangedEvent) (err error) {
	start := time.Now()
	defer func() { inframetrics.ObserveWorkerJob("interaction_action_changed", time.Since(start), err) }()
	if err = validateActionEvent(event); err != nil {
		return err
	}
	if err = w.invalidator.InvalidateVideoStats(ctx, event.VideoID); err != nil {
		return err
	}
	return w.effects.ProjectActionEffects(ctx, event)
}
