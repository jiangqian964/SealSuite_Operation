package scheduler

import (
	"testing"
	"time"
)

func TestSchedulerAcceptsSecondsSpec(t *testing.T) {
	s := New(time.Local)
	if err := s.AddJob("t", "0 */1 * * * *", func() error { return nil }); err != nil {
		t.Fatalf("expected seconds spec accepted, got err=%v", err)
	}
}

