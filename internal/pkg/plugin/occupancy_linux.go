//go:build linux

/**
 * Copyright 2026 Advanced Micro Devices, Inc. All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 */

package plugin

import "syscall"

func defaultKill(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}
