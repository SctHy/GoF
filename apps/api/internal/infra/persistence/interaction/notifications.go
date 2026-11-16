package infrainteraction

import (
	app "GCFeed/internal/application/interaction"
	domain "GCFeed/internal/domain/interaction"
	domainmessage "GCFeed/internal/domain/message"
	inframessage "GCFeed/internal/infra/persistence/message"
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
)

// ProjectActionNotification 对同一用户给同一视频的点赞只生成一条通知。
func (r *Repository) ProjectActionNotification(ctx context.Context, event *app.ActionChangedEvent) error {
	if event == nil || event.EffectsVersion != 1 || event.Delta <= 0 || event.ActionType != domain.ActionTypeLike {
		return nil
	}
	var video struct{ AuthorID int64 }
	// 事件提交后视频可能软删除；不要将合法的历史事件变成永久失败。
	err := r.db.WithContext(ctx).Table("video").Select("author_id").Where("id = ?", event.VideoID).Take(&video).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if video.AuthorID == event.UserID {
		return nil
	}
	actor, err := r.GetUserProfile(ctx, event.UserID)
	if err != nil && !errors.Is(err, domain.ErrInvalidUserID) {
		return err
	}
	eventID := fmt.Sprintf("interaction:like:%d:%d", event.VideoID, event.UserID)
	message, err := domainmessage.New(video.AuthorID, domainmessage.TypeLike, "收到点赞", "点赞了你的视频", eventID)
	if err != nil {
		return err
	}
	nickname, avatar := "", ""
	if actor != nil {
		nickname, avatar = actor.Nickname, actor.AvatarURL
	}
	message.WithActor(event.UserID, nickname, avatar)
	_, _, err = inframessage.New(r.db).Create(ctx, message, eventID)
	return err
}
