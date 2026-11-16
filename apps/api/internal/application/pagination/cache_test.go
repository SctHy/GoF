package pagination

import (
	"context"
	"testing"
	"time"
)

func TestBoundedCacheExpiryAndIsolation(t *testing.T) {
	c := NewMemoryCache(2)
	now := time.Now()
	c.now = func() time.Time { return now }
	original := []byte("one")
	_ = c.WriteBlob(context.Background(), "a", original, time.Second)
	original[0] = 'x'
	value, ok, err := c.ReadBlob(context.Background(), "a")
	if err != nil || !ok || string(value) != "one" {
		t.Fatal("cache did not own content")
	}
	value[0] = 'x'
	value, _, _ = c.ReadBlob(context.Background(), "a")
	if string(value) != "one" {
		t.Fatal("read leaked mutable content")
	}
	_ = c.WriteBlob(context.Background(), "b", []byte("two"), time.Second)
	_ = c.WriteBlob(context.Background(), "c", []byte("three"), time.Second)
	if _, found, _ := c.ReadBlob(context.Background(), "a"); found {
		t.Fatal("LRU capacity not enforced")
	}
	now = now.Add(time.Second)
	if _, found, _ := c.ReadBlob(context.Background(), "c"); found {
		t.Fatal("expired cache entry returned")
	}
}
func TestSnapshotCursorValidation(t *testing.T) {
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := Decode(Encode(id, 10))
	if err != nil || cursor.ID != id || cursor.Offset != 10 {
		t.Fatal("cursor roundtrip failed")
	}
	for _, raw := range []string{"old-cursor", "s1.bad", Encode("no", 0), Encode(id, -1), Encode(id, MaxItems+1)} {
		if _, err := Decode(raw); err == nil {
			t.Fatalf("accepted invalid cursor %q", raw)
		}
	}
	if cursor, err := Decode(""); cursor != nil || err != nil {
		t.Fatal("empty cursor rejected")
	}
}
