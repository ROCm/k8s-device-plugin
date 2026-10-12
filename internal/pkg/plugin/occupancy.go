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
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
)

const (
	kfdProcRoot  = "/sys/class/kfd/kfd/proc"
	hostProcRoot = "/proc"
	// DefaultReclaimTimeout is how long reclaim waits for VRAM to return to zero.
	DefaultReclaimTimeout = 60 * time.Second
	// DefaultReclaimCooldown is how long a GPU stays unschedulable after reclaim times out.
	DefaultReclaimCooldown = 60 * time.Second
	reclaimTermGrace       = 2 * time.Second
	reclaimPollInterval    = 200 * time.Millisecond
)

// kfdOccupant is one host process holding VRAM on a GPU.
type kfdOccupant struct {
	pid   int
	bytes uint64
	comm  string
}

// reclaimTimeoutError means signaling did not free the GPU before the deadline.
type reclaimTimeoutError struct {
	deviceID  string
	remaining []kfdOccupant
}

func (e *reclaimTimeoutError) Error() string {
	var total uint64
	for _, occupant := range e.remaining {
		total += occupant.bytes
	}
	return fmt.Sprintf("GPU %s reclaim timed out with residual KFD VRAM (%d bytes across %d process(es))", e.deviceID, total, len(e.remaining))
}

// kfdInspector reads KFD per-process VRAM counters and, when reclaim is
// enabled, signals the processes that hold them. The filesystem roots, clock,
// and signal function are replaceable so tests do not touch the host.
type kfdInspector struct {
	procRoot string
	procFS   string
	signal   func(pid int, sig syscall.Signal) error
	sleep    func(time.Duration)
	now      func() time.Time
	logf     func(string, ...interface{})
	selfPID  int
	grace    time.Duration
	poll     time.Duration
}

func newKFDInspector() *kfdInspector {
	return &kfdInspector{
		procRoot: kfdProcRoot,
		procFS:   hostProcRoot,
		signal:   defaultKill,
		sleep:    time.Sleep,
		now:      time.Now,
		logf:     glog.Infof,
		selfPID:  os.Getpid(),
		grace:    reclaimTermGrace,
		poll:     reclaimPollInterval,
	}
}

// prepare logs every process holding VRAM on the GPU. With reclaim disabled it
// fails closed. With reclaim enabled it signals those processes and waits
// until the counters drop to zero or the timeout expires.
func (k *kfdInspector) prepare(deviceID string, gpuNodeID int, reclaim bool, timeout time.Duration) error {
	if k.logf == nil {
		k.logf = glog.Infof
	}
	occupants, err := k.listOccupants(gpuNodeID)
	if err != nil {
		return fmt.Errorf("GPU %s: %w", deviceID, err)
	}
	if len(occupants) == 0 {
		k.logf("GPU %s KFD gpu_id %d is free and ready", deviceID, gpuNodeID)
		return nil
	}
	k.logOccupants(deviceID, gpuNodeID, occupants, "occupying VRAM")
	if !reclaim {
		return fmt.Errorf("GPU %s is occupied by residual KFD VRAM (%d bytes)", deviceID, vramTotal(occupants))
	}
	if timeout <= 0 {
		timeout = DefaultReclaimTimeout
	}
	if k.now == nil {
		k.now = time.Now
	}
	if k.sleep == nil {
		k.sleep = time.Sleep
	}
	if k.signal == nil {
		k.signal = defaultKill
	}
	if k.grace <= 0 {
		k.grace = reclaimTermGrace
	}
	if k.poll <= 0 {
		k.poll = reclaimPollInterval
	}

	start := k.now()
	deadline := start.Add(timeout)
	killAt := start.Add(k.grace)
	if killAt.After(deadline) {
		killAt = deadline
	}
	k.signalOccupants(deviceID, gpuNodeID, occupants, syscall.SIGTERM)

	killed := false
	for {
		occupants, err = k.listOccupants(gpuNodeID)
		if err != nil {
			return fmt.Errorf("GPU %s: %w", deviceID, err)
		}
		if len(occupants) == 0 {
			k.logf("GPU %s KFD gpu_id %d is free and ready", deviceID, gpuNodeID)
			return nil
		}
		now := k.now()
		if !killed && !now.Before(killAt) {
			k.logOccupants(deviceID, gpuNodeID, occupants, "still occupying VRAM after SIGTERM")
			k.signalOccupants(deviceID, gpuNodeID, occupants, syscall.SIGKILL)
			killed = true
			continue
		}
		if !now.Before(deadline) {
			k.logOccupants(deviceID, gpuNodeID, occupants, "still occupying VRAM after reclaim timeout")
			return &reclaimTimeoutError{deviceID: deviceID, remaining: occupants}
		}

		step := k.poll
		remaining := deadline.Sub(now)
		if !killed {
			untilKill := killAt.Sub(now)
			if untilKill > 0 && untilKill < step {
				step = untilKill
			}
		}
		if step > remaining {
			step = remaining
		}
		if step > 0 {
			k.sleep(step)
		}
	}
}

func (k *kfdInspector) listOccupants(gpuNodeID int) ([]kfdOccupant, error) {
	processes, err := os.ReadDir(k.procRoot)
	if err != nil {
		return nil, fmt.Errorf("cannot inspect KFD process state: %w", err)
	}
	// gpuNodeID is the KFD gpu_id from topology/nodes/<n>/gpu_id. The VRAM
	// counter is vram_<gpu_id>, not vram_<topology node index>.
	counter := fmt.Sprintf("vram_%d", gpuNodeID)
	var occupants []kfdOccupant
	for _, process := range processes {
		if !process.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(process.Name())
		if err != nil {
			continue
		}
		path := filepath.Join(k.procRoot, process.Name(), counter)
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read KFD occupancy counter %s: %w", counter, err)
		}
		bytes, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("cannot parse KFD occupancy counter %s: %w", counter, err)
		}
		if bytes == 0 {
			continue
		}
		occupants = append(occupants, kfdOccupant{
			pid:   pid,
			bytes: bytes,
			comm:  k.processComm(pid),
		})
	}
	sort.Slice(occupants, func(i, j int) bool {
		return occupants[i].pid < occupants[j].pid
	})
	return occupants, nil
}

func (k *kfdInspector) processComm(pid int) string {
	data, err := os.ReadFile(filepath.Join(k.procFS, strconv.Itoa(pid), "comm"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (k *kfdInspector) logOccupants(deviceID string, gpuNodeID int, occupants []kfdOccupant, why string) {
	for _, occupant := range occupants {
		k.logf("GPU %s KFD gpu_id %d pid %d comm %q holds %d bytes of VRAM (%s)", deviceID, gpuNodeID, occupant.pid, occupant.comm, occupant.bytes, why)
	}
}

func (k *kfdInspector) signalOccupants(deviceID string, gpuNodeID int, occupants []kfdOccupant, sig syscall.Signal) {
	for _, occupant := range occupants {
		if occupant.pid <= 1 || occupant.pid == k.selfPID {
			k.logf("GPU %s KFD gpu_id %d pid %d comm %q holds %d bytes of VRAM; not signaling protected process", deviceID, gpuNodeID, occupant.pid, occupant.comm, occupant.bytes)
			continue
		}
		k.logf("GPU %s KFD gpu_id %d signaling pid %d comm %q with %s", deviceID, gpuNodeID, occupant.pid, occupant.comm, sig)
		if err := k.signal(occupant.pid, sig); err != nil {
			k.logf("GPU %s failed to signal pid %d with %s: %v", deviceID, occupant.pid, sig, err)
		}
	}
}

func vramTotal(occupants []kfdOccupant) uint64 {
	var total uint64
	for _, occupant := range occupants {
		total += occupant.bytes
	}
	return total
}

// ValidateReclaimTimeout reports an error when the reclaim wait is not a positive duration.
func ValidateReclaimTimeout(timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("occupancy_reclaim_timeout must be greater than 0")
	}
	return nil
}
