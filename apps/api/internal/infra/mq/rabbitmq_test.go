package inframq

import (
	"context"
	"errors"
	"testing"
)

func TestRetryHandlerBounded(t *testing.T) {
	expected := errors.New("poison message")
	calls := 0
	err := retryHandler(context.Background(), func() error { calls++; return expected })
	if !errors.Is(err, expected) || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestRetryHandlerRecovers(t *testing.T) {
	calls := 0
	err := retryHandler(context.Background(), func() error {
		calls++
		if calls == 1 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestRetryHandlerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := retryHandler(ctx, func() error { calls++; cancel(); return errors.New("interrupted") })
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
