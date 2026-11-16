package infracache

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestSharedSnapshotExpiryAndBound(t *testing.T) {
	client := isolatedRedis(t)
	first, second := NewFeedCache(client), NewFeedCache(client)
	ctx := context.Background()
	if err := first.WriteBlob(ctx, "short", []byte("snapshot"), time.Second); err != nil {
		t.Fatal(err)
	}
	value, found, err := second.ReadBlob(ctx, "short")
	if err != nil || !found || string(value) != "snapshot" {
		t.Fatal("snapshot is not shared across instances")
	}
	if err := client.PExpire(ctx, blobKeyPrefix+"short", time.Millisecond).Err(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, found, err := second.ReadBlob(ctx, "short"); err != nil || found {
		t.Fatal("expired snapshot survived")
	}
	for i := 0; i < maxSharedBlobs+1; i++ {
		if err := first.WriteBlob(ctx, fmt.Sprintf("capacity:%04d", i), []byte("data"), time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	if n := client.ZCard(ctx, blobIndexKey).Val(); n != maxSharedBlobs {
		t.Fatalf("registry count=%d", n)
	}
	if n := client.DBSize(ctx).Val(); n > maxSharedBlobs+1 {
		t.Fatalf("unbounded blob keys: %d", n)
	}
}
