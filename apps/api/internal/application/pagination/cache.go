// Package pagination provides bounded short-lived storage for immutable ranked pages.
package pagination

import (
	"container/list"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
)

const SnapshotTTL = 2 * time.Minute
const MaxItems = 500
const MaxBlobBytes = 512 << 10

var ErrInvalidCursor = errors.New("invalid or expired snapshot cursor")

// Cache supports a local bounded cache or shared Redis without coupling ranking to Redis.
type Cache interface {
	ReadBlob(context.Context, string) ([]byte, bool, error)
	WriteBlob(context.Context, string, []byte, time.Duration) error
	DeleteBlob(context.Context, string) error
}

type Cursor struct {
	ID     string `json:"id"`
	Offset int    `json:"offset"`
}

func NewID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(id[:]), nil
}
func Encode(id string, offset int) string {
	content, _ := json.Marshal(Cursor{ID: id, Offset: offset})
	return "s1." + base64.RawURLEncoding.EncodeToString(content)
}

// Decode accepts only the current snapshot format; an empty cursor starts a page.
func Decode(raw string) (*Cursor, error) {
	if raw == "" {
		return nil, nil
	}
	if !strings.HasPrefix(raw, "s1.") {
		return nil, ErrInvalidCursor
	}
	content, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(raw, "s1."))
	if err != nil {
		return nil, ErrInvalidCursor
	}
	var cursor Cursor
	if err := json.Unmarshal(content, &cursor); err != nil {
		return nil, ErrInvalidCursor
	}
	id, err := hex.DecodeString(cursor.ID)
	if err != nil || len(id) != 16 || cursor.Offset < 0 || cursor.Offset > MaxItems {
		return nil, ErrInvalidCursor
	}
	return &cursor, nil
}

type entry struct {
	key     string
	content []byte
	expires time.Time
}
type MemoryCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*list.Element
	order    *list.List
	now      func() time.Time
}

func NewMemoryCache(capacity int) *MemoryCache {
	if capacity < 1 {
		capacity = 1
	}
	return &MemoryCache{capacity: capacity, items: make(map[string]*list.Element), order: list.New(), now: time.Now}
}
func (c *MemoryCache) ReadBlob(ctx context.Context, key string) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.items[key]
	if e == nil {
		return nil, false, nil
	}
	v := e.Value.(*entry)
	if !c.now().Before(v.expires) {
		delete(c.items, key)
		c.order.Remove(e)
		return nil, false, nil
	}
	c.order.MoveToBack(e)
	return append([]byte(nil), v.content...), true, nil
}
func (c *MemoryCache) WriteBlob(ctx context.Context, key string, content []byte, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(content) > MaxBlobBytes || ttl <= 0 {
		return errors.New("invalid snapshot size or TTL")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if old := c.items[key]; old != nil {
		c.order.Remove(old)
		delete(c.items, key)
	}
	for len(c.items) >= c.capacity {
		old := c.order.Front()
		delete(c.items, old.Value.(*entry).key)
		c.order.Remove(old)
	}
	c.items[key] = c.order.PushBack(&entry{key: key, content: append([]byte(nil), content...), expires: c.now().Add(ttl)})
	return nil
}
func (c *MemoryCache) DeleteBlob(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.items[key]; e != nil {
		delete(c.items, key)
		c.order.Remove(e)
	}
	return nil
}
