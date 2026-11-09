//go:build linux

/**
 * Copyright 2026 Advanced Micro Devices, Inc. All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 */

package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

// defaultKill signals pid only while pidfd_open still refers to the process
// whose starttime was captured with the KFD scan. A recycled pid fails the
// starttime check and is not signaled. There is no fallback to kill(2).
func defaultKill(pid int, sig syscall.Signal, startTicks uint64) error {
	if pid <= 1 || startTicks == 0 {
		return fmt.Errorf("refusing to signal pid %d without a confirmed start time", pid)
	}
	fd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return fmt.Errorf("pidfd_open %d: %w", pid, err)
	}
	defer unix.Close(fd)

	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return fmt.Errorf("pid %d identity changed before signal: %w", pid, err)
	}
	_, got, err := parseProcStat(string(data))
	if err != nil || got != startTicks {
		return fmt.Errorf("pid %d identity changed before signal", pid)
	}
	if err := unix.PidfdSendSignal(fd, unix.Signal(sig), nil, 0); err != nil {
		return fmt.Errorf("pidfd_send_signal %d: %w", pid, err)
	}
	return nil
}
