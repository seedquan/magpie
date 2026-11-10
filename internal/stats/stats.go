// Package stats preserves startup compatibility without collecting or sending
// third-party usage telemetry. This fork cannot opt in through settings or env.
package stats

import (
	"context"
	"time"
)

// Off is always true in this fork.
func Off() bool { return true }

// Run returns immediately; no telemetry worker is started.
func Run(version, what string) {}

// Send does nothing, including when a legacy caller invokes it directly.
func Send(ctx context.Context, version, what string, now time.Time) error { return nil }
