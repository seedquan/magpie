package plugin

import (
	"context"
	"testing"
	"time"
)

func TestForkBackgroundUpdatersDisabled(t *testing.T) {
	// No Bun required: disabled schedulers must return without any runtime.
	for _, run := range []struct {
		name string
		f    func(context.Context)
	}{{"plugins", KeepUpdated}, {"bun", KeepBunUpdated}} {
		t.Run(run.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			done := make(chan struct{})
			go func() { defer close(done); run.f(ctx) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Error("background updater still schedules work")
			}
			cancel()
			<-done
		})
	}
}
