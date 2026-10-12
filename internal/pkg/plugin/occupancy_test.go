/**
 * Copyright 2026 Advanced Micro Devices, Inc. All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 */

package plugin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/ROCm/k8s-device-plugin/internal/pkg/allocator"
	"golang.org/x/net/context"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Sleep(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type recordedSignal struct {
	pid int
	sig syscall.Signal
}

func writeOccupant(t *testing.T, procRoot, procFS string, pid int, nodeID int, vram uint64, comm string) {
	t.Helper()
	procDir := filepath.Join(procRoot, strconv.Itoa(pid))
	if err := os.MkdirAll(procDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(procDir, fmt.Sprintf("vram_%d", nodeID)), []byte(strconv.FormatUint(vram, 10)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commDir := filepath.Join(procFS, strconv.Itoa(pid))
	if err := os.MkdirAll(commDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commDir, "comm"), []byte(comm+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testInspector(t *testing.T) (*kfdInspector, *fakeClock, *[]string, *[]recordedSignal) {
	t.Helper()
	root := t.TempDir()
	procRoot := filepath.Join(root, "kfd")
	procFS := filepath.Join(root, "proc")
	if err := os.MkdirAll(procRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(procRoot, "not-a-process"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	logs := &[]string{}
	signals := &[]recordedSignal{}
	inspector := newKFDInspector()
	inspector.procRoot = procRoot
	inspector.procFS = procFS
	inspector.now = clock.Now
	inspector.sleep = clock.Sleep
	inspector.selfPID = 42
	inspector.grace = 2 * time.Second
	inspector.poll = 200 * time.Millisecond
	inspector.logf = func(format string, args ...interface{}) {
		*logs = append(*logs, fmt.Sprintf(format, args...))
	}
	inspector.signal = func(pid int, sig syscall.Signal) error {
		*signals = append(*signals, recordedSignal{pid: pid, sig: sig})
		return nil
	}
	return inspector, clock, logs, signals
}

func TestPrepareAlreadyFree(t *testing.T) {
	inspector, _, logs, signals := testInspector(t)
	writeOccupant(t, inspector.procRoot, inspector.procFS, 100, 1, 0, "idle")
	writeOccupant(t, inspector.procRoot, inspector.procFS, 101, 2, 4096, "other-gpu")

	if err := inspector.prepare("gpuA", 1, true, 20*time.Second); err != nil {
		t.Fatalf("free GPU returned error: %v", err)
	}
	if len(*signals) != 0 {
		t.Fatalf("signaled processes on a free GPU: %+v", *signals)
	}
	if !strings.Contains(strings.Join(*logs, "\n"), "free and ready") {
		t.Fatalf("logs = %v, want a free confirmation", *logs)
	}
}

func TestPrepareLogsThenKillsThenConfirmsFree(t *testing.T) {
	inspector, _, logs, signals := testInspector(t)
	writeOccupant(t, inspector.procRoot, inspector.procFS, 100, 1, 8192, "leaked-job")
	inspector.signal = func(pid int, sig syscall.Signal) error {
		*signals = append(*signals, recordedSignal{pid: pid, sig: sig})
		if sig == syscall.SIGKILL {
			if err := os.Remove(filepath.Join(inspector.procRoot, strconv.Itoa(pid), "vram_1")); err != nil {
				t.Errorf("remove vram counter: %v", err)
			}
		}
		return nil
	}

	if err := inspector.prepare("gpuA", 1, true, 20*time.Second); err != nil {
		t.Fatalf("reclaim returned error: %v", err)
	}
	if len(*signals) != 2 || (*signals)[0].sig != syscall.SIGTERM || (*signals)[1].sig != syscall.SIGKILL {
		t.Fatalf("signals = %+v, want SIGTERM then SIGKILL", *signals)
	}
	joined := strings.Join(*logs, "\n")
	logAt := strings.Index(joined, `pid 100 comm "leaked-job" holds 8192 bytes of VRAM (occupying VRAM)`)
	if logAt < 0 {
		t.Fatalf("logs = %v, want the occupant recorded before signaling", *logs)
	}
	if !strings.Contains(joined, "free and ready") {
		t.Fatalf("logs = %v, want the GPU confirmed free", *logs)
	}
}

func TestPrepareDoesNotSignalProtectedOrCheckOnly(t *testing.T) {
	inspector, _, logs, signals := testInspector(t)
	writeOccupant(t, inspector.procRoot, inspector.procFS, 1, 1, 100, "init")
	writeOccupant(t, inspector.procRoot, inspector.procFS, 42, 1, 200, "device-plugin")
	writeOccupant(t, inspector.procRoot, inspector.procFS, 100, 1, 300, "leaked-job")

	err := inspector.prepare("gpuA", 1, false, time.Second)
	if err == nil {
		t.Fatal("check-only mode succeeded while VRAM was allocated")
	}
	if len(*signals) != 0 {
		t.Fatalf("check-only signaled processes: %+v", *signals)
	}
	if !strings.Contains(strings.Join(*logs, "\n"), "pid 100") {
		t.Fatalf("logs = %v, want every occupant logged", *logs)
	}

	*logs = nil
	inspector.signal = func(pid int, sig syscall.Signal) error {
		*signals = append(*signals, recordedSignal{pid: pid, sig: sig})
		if pid == 1 || pid == 42 {
			t.Errorf("signaled protected pid %d", pid)
		}
		if sig == syscall.SIGKILL && pid == 100 {
			os.Remove(filepath.Join(inspector.procRoot, "100", "vram_1"))
		}
		return nil
	}
	err = inspector.prepare("gpuA", 1, true, time.Second)
	var timeout *reclaimTimeoutError
	if !errors.As(err, &timeout) {
		t.Fatalf("error = %v, want reclaim timeout while protected processes remain", err)
	}
	for _, signal := range *signals {
		if signal.pid == 1 || signal.pid == 42 {
			t.Fatalf("signaled protected pid %d", signal.pid)
		}
	}
	if len(*signals) == 0 || (*signals)[0].pid != 100 {
		t.Fatalf("signals = %+v, want pid 100 signaled", *signals)
	}
}

func TestPrepareTimeoutWhenKillDoesNotFreeVRAM(t *testing.T) {
	inspector, _, _, signals := testInspector(t)
	inspector.grace = 200 * time.Millisecond
	writeOccupant(t, inspector.procRoot, inspector.procFS, 100, 1, 4096, "stuck")

	err := inspector.prepare("gpuA", 1, true, time.Second)
	var timeout *reclaimTimeoutError
	if !errors.As(err, &timeout) {
		t.Fatalf("error = %v, want reclaim timeout", err)
	}
	if timeout.deviceID != "gpuA" || len(timeout.remaining) != 1 || timeout.remaining[0].pid != 100 {
		t.Fatalf("timeout = %+v", timeout)
	}
	sawTerm, sawKill := false, false
	for _, signal := range *signals {
		if signal.pid == 100 && signal.sig == syscall.SIGTERM {
			sawTerm = true
		}
		if signal.pid == 100 && signal.sig == syscall.SIGKILL {
			sawKill = true
		}
	}
	if !sawTerm || !sawKill {
		t.Fatalf("signals = %+v, want SIGTERM and SIGKILL before timeout", *signals)
	}
}

type recordingPolicy struct {
	available []string
}

func (r *recordingPolicy) Init([]*allocator.Device, string) error { return nil }

func (r *recordingPolicy) Allocate(available, required []string, size int) ([]string, error) {
	r.available = append([]string(nil), available...)
	if len(available) < size {
		return nil, fmt.Errorf("not enough devices")
	}
	return available[:size], nil
}

func gpuMap(ids ...string) map[string]map[string]interface{} {
	devices := make(map[string]map[string]interface{}, len(ids))
	for i, id := range ids {
		devices[id] = map[string]interface{}{
			"nodeId":  i + 1,
			"card":    i,
			"renderD": 128 + i,
		}
	}
	return devices
}

func TestAllocateReclaimUsesKFDGPUId(t *testing.T) {
	inspector, _, logs, signals := testInspector(t)
	// vram_<node index> is not the counter the kernel publishes.
	writeOccupant(t, inspector.procRoot, inspector.procFS, 100, 8, 4096, "node-index")
	writeOccupant(t, inspector.procRoot, inspector.procFS, 200, 30939, 8192, "gpu-id")
	inspector.signal = func(pid int, sig syscall.Signal) error {
		*signals = append(*signals, recordedSignal{pid: pid, sig: sig})
		if sig == syscall.SIGKILL && pid == 200 {
			if err := os.Remove(filepath.Join(inspector.procRoot, "200", "vram_30939")); err != nil {
				t.Errorf("remove vram counter: %v", err)
			}
		}
		return nil
	}

	plug := NewAMDGPUPlugin(WithOccupancyReclaim(true, time.Second, time.Minute))
	plug.inspector = inspector
	plug.AMDGPUs = map[string]map[string]interface{}{
		"0000:8e:00.0": {"nodeId": 8, "gpuId": 30939, "card": 7, "renderD": 134},
	}

	_, err := plug.Allocate(context.Background(), &pluginapi.AllocateRequest{
		ContainerRequests: []*pluginapi.ContainerAllocateRequest{{
			DevicesIDs: []string{"0000:8e:00.0"},
		}},
	})
	if err != nil {
		t.Fatalf("allocate returned error: %v", err)
	}
	for _, signal := range *signals {
		if signal.pid == 100 {
			t.Fatalf("signaled the topology-index counter occupant: %+v", *signals)
		}
	}
	if len(*signals) != 2 || (*signals)[0].pid != 200 || (*signals)[1].pid != 200 {
		t.Fatalf("signals = %+v, want SIGTERM and SIGKILL for the gpu_id occupant", *signals)
	}
	if !strings.Contains(strings.Join(*logs, "\n"), "gpu_id 30939") {
		t.Fatalf("logs = %v, want the KFD gpu_id", *logs)
	}
}

func TestAllocateReclaimTimeoutQuarantinesGPU(t *testing.T) {
	inspector, _, _, _ := testInspector(t)
	inspector.grace = 200 * time.Millisecond
	writeOccupant(t, inspector.procRoot, inspector.procFS, 100, 1, 4096, "stuck")

	plugin := NewAMDGPUPlugin(WithOccupancyReclaim(true, time.Second, time.Minute))
	plugin.inspector = inspector
	plugin.AMDGPUs = gpuMap("gpuA", "gpuB")

	_, err := plugin.Allocate(context.Background(), &pluginapi.AllocateRequest{
		ContainerRequests: []*pluginapi.ContainerAllocateRequest{{
			DevicesIDs: []string{"gpuA"},
		}},
	})
	if err == nil {
		t.Fatal("Allocate succeeded while VRAM reclaim timed out")
	}
	if !plugin.isQuarantined("gpuA") {
		t.Fatal("timed-out GPU was not quarantined")
	}
	if plugin.isQuarantined("gpuB") {
		t.Fatal("unrelated GPU was quarantined")
	}

	devs := []*pluginapi.Device{
		{ID: "gpuA", Health: pluginapi.Healthy},
		{ID: "gpuB", Health: pluginapi.Healthy},
	}
	plugin.applyQuarantine(devs)
	if devs[0].Health != pluginapi.Unhealthy {
		t.Fatalf("quarantined GPU health = %s", devs[0].Health)
	}
	if devs[1].Health != pluginapi.Healthy {
		t.Fatalf("other GPU health = %s", devs[1].Health)
	}
}

func TestCheckOnlyDoesNotQuarantineOrSignal(t *testing.T) {
	inspector, _, _, signals := testInspector(t)
	writeOccupant(t, inspector.procRoot, inspector.procFS, 100, 1, 4096, "stuck")

	plugin := NewAMDGPUPlugin(WithOccupancyCheck(true))
	plugin.inspector = inspector
	plugin.AMDGPUs = gpuMap("gpuA")

	_, err := plugin.PreStartContainer(context.Background(), &pluginapi.PreStartContainerRequest{
		DevicesIDs: []string{"gpuA"},
	})
	if err == nil {
		t.Fatal("check-only PreStart succeeded while VRAM was allocated")
	}
	if len(*signals) != 0 {
		t.Fatalf("check-only signaled processes: %+v", *signals)
	}
	if plugin.isQuarantined("gpuA") {
		t.Fatal("check-only quarantined the GPU")
	}
}

func TestPreferredAllocationSkipsQuarantinedGPU(t *testing.T) {
	inspector, _, _, _ := testInspector(t)
	writeOccupant(t, inspector.procRoot, inspector.procFS, 100, 1, 4096, "stuck")
	policy := &recordingPolicy{}
	plugin := NewAMDGPUPlugin(
		WithAllocator(policy),
		WithOccupancyReclaim(true, time.Second, time.Minute),
	)
	plugin.inspector = inspector
	plugin.AMDGPUs = gpuMap("gpuA", "gpuB")
	plugin.quarantineGPU("gpuA")

	response, err := plugin.GetPreferredAllocation(context.Background(), &pluginapi.PreferredAllocationRequest{
		ContainerRequests: []*pluginapi.ContainerPreferredAllocationRequest{{
			AvailableDeviceIDs: []string{"gpuA", "gpuB"},
			AllocationSize:     1,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(policy.available) != 1 || policy.available[0] != "gpuB" {
		t.Fatalf("allocator saw %v, want [gpuB]", policy.available)
	}
	if got := response.ContainerResponses[0].DeviceIDs; len(got) != 1 || got[0] != "gpuB" {
		t.Fatalf("preferred devices = %v", got)
	}

	policy.available = nil
	response, err = plugin.GetPreferredAllocation(context.Background(), &pluginapi.PreferredAllocationRequest{
		ContainerRequests: []*pluginapi.ContainerPreferredAllocationRequest{{
			AvailableDeviceIDs:   []string{"gpuA", "gpuB"},
			MustIncludeDeviceIDs: []string{"gpuA"},
			AllocationSize:       1,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(policy.available) != 2 {
		t.Fatalf("must-include quarantined GPU was dropped: %v", policy.available)
	}
	if got := response.ContainerResponses[0].DeviceIDs; len(got) != 1 || got[0] != "gpuA" {
		t.Fatalf("preferred devices = %v, want must-include gpuA retained", got)
	}
}

func TestQuarantineBlocksAnotherReclaimUntilVRAMIsFree(t *testing.T) {
	inspector, _, _, signals := testInspector(t)
	writeOccupant(t, inspector.procRoot, inspector.procFS, 100, 1, 4096, "stuck")
	plugin := NewAMDGPUPlugin(WithOccupancyReclaim(true, time.Second, time.Minute))
	plugin.inspector = inspector
	plugin.AMDGPUs = gpuMap("gpuA")
	plugin.quarantineGPU("gpuA")

	err := plugin.prepareRequestedGPUs([]string{"gpuA"})
	if err == nil {
		t.Fatal("quarantined GPU was prepared again")
	}
	if len(*signals) != 0 {
		t.Fatalf("reclaim ran again while the GPU was quarantined: %+v", *signals)
	}

	if err := os.Remove(filepath.Join(inspector.procRoot, "100", "vram_1")); err != nil {
		t.Fatal(err)
	}
	plugin.expireQuarantine(time.Now())
	if plugin.isQuarantined("gpuA") {
		t.Fatal("quarantine remained after VRAM read as free")
	}
}

func TestQuarantineExpiresAfterCooldown(t *testing.T) {
	plugin := NewAMDGPUPlugin(WithOccupancyReclaim(true, time.Second, time.Minute))
	plugin.quarantine["gpuA"] = time.Now().Add(-time.Second)
	plugin.expireQuarantine(time.Now())
	if plugin.isQuarantined("gpuA") {
		t.Fatal("quarantine remained after the cooldown")
	}
}
