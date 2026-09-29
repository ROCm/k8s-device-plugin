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
	"strings"
)

const kfdProcRoot = "/sys/class/kfd/kfd/proc"

// checkGPUOccupancy checks KFD's per-process VRAM counters for a GPU. KFD
// names the counter with its GPU node ID (vram_<id>). A missing counter means
// that process has no KFD allocation on this GPU; inability to inspect KFD is
// an error so callers can fail closed.
func checkGPUOccupancy(deviceID string, gpuNodeID int) error {
	processes, err := os.ReadDir(kfdProcRoot)
	if err != nil {
		return fmt.Errorf("cannot inspect KFD process state: %w", err)
	}
	counter := fmt.Sprintf("vram_%d", gpuNodeID)
	for _, process := range processes {
		if !process.IsDir() {
			continue
		}
		path := filepath.Join(kfdProcRoot, process.Name(), counter)
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("cannot read KFD occupancy for GPU %s: %w", deviceID, err)
		}
		bytes, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
		if err != nil {
			return fmt.Errorf("cannot parse KFD occupancy for GPU %s: %w", deviceID, err)
		}
		if bytes > 0 {
			return fmt.Errorf("GPU %s is occupied by residual KFD VRAM (%d bytes)", deviceID, bytes)
		}
	}
	return nil
}
