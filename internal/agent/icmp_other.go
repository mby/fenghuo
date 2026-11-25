//go:build !linux

package agent

import "context"

func icmpLatency(context.Context, string) (float64, string) { return -1, "unsupported" }
