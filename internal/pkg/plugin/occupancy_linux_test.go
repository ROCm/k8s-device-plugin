//go:build linux

/**
 * Copyright 2026 Advanced Micro Devices, Inc. All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 */

package plugin

import (
	"os"
	"syscall"
	"testing"
)

func TestDefaultKillRefusesMismatchedStartTime(t *testing.T) {
	if err := defaultKill(os.Getpid(), syscall.SIGCONT, ^uint64(0)); err == nil {
		t.Fatal("pidfd signal accepted a mismatched start time")
	}
	if err := defaultKill(os.Getpid(), syscall.SIGCONT, 0); err == nil {
		t.Fatal("pidfd signal accepted an unconfirmed start time")
	}
}
