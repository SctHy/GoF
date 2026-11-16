package test

import (
	interaction "GCFeed/internal/domain/interaction"
	relation "GCFeed/internal/domain/relation"
	video "GCFeed/internal/domain/video"
	"testing"
	"time"
)

func TestRestoringUnknownStatusDoesNotActivateRecords(t *testing.T) {
	now := time.Now()
	for _, status := range []int{0, -1} {
		action := interaction.RestoreAction(1, 2, 3, "LIKE", status, "", now, now)
		follow := relation.RestoreFollow(1, 2, 3, status, "", now, now)
		comment := interaction.RestoreComment(1, 2, 3, "user", "", "text", status, "", now, now)
		clip := video.RestoreVideo(1, 2, "title", "", "/video", "/cover", status, 0, 0, 0, &now, now, now, "")
		if action.Active() || follow.Active() || comment.Status != status || clip.Status != status {
			t.Fatalf("restoring status %d activated a record", status)
		}
	}
}
