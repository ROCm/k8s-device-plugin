//go:build !linux

/**
 * Copyright 2026 Advanced Micro Devices, Inc. All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 */

package plugin

import (
	"fmt"
	"syscall"
)

func defaultKill(pid int, sig syscall.Signal) error {
	return fmt.Errorf("signaling pid %d with %s is only supported on linux", pid, sig)
}
