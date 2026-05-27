/**
 * Copyright 2018 Advanced Micro Devices, Inc.  All rights reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
**/

package plugin

import (
	"testing"

	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

func TestBuildVirtualDevices_Replicas1(t *testing.T) {
	physicalIDs := []string{"0000:03:00.0", "0000:04:00.0"}
	devs := buildVirtualDevices(physicalIDs, 1)

	if len(devs) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devs))
	}

	expected := []string{"0000:03:00.0-slice-0", "0000:04:00.0-slice-0"}
	for i, dev := range devs {
		if dev.ID != expected[i] {
			t.Errorf("device %d: expected ID %q, got %q", i, expected[i], dev.ID)
		}
		if dev.Health != pluginapi.Healthy {
			t.Errorf("device %d: expected health %q, got %q", i, pluginapi.Healthy, dev.Health)
		}
	}
}

func TestBuildVirtualDevices_Replicas4(t *testing.T) {
	physicalIDs := []string{"0000:03:00.0", "0000:04:00.0"}
	devs := buildVirtualDevices(physicalIDs, 4)

	if len(devs) != 8 {
		t.Fatalf("expected 8 devices, got %d", len(devs))
	}

	// Verify all IDs are unique
	seen := make(map[string]bool)
	for _, dev := range devs {
		if seen[dev.ID] {
			t.Errorf("duplicate device ID: %s", dev.ID)
		}
		seen[dev.ID] = true
	}

	// Verify expected IDs
	expectedIDs := map[string]bool{
		"0000:03:00.0-slice-0": true,
		"0000:03:00.0-slice-1": true,
		"0000:03:00.0-slice-2": true,
		"0000:03:00.0-slice-3": true,
		"0000:04:00.0-slice-0": true,
		"0000:04:00.0-slice-1": true,
		"0000:04:00.0-slice-2": true,
		"0000:04:00.0-slice-3": true,
	}
	for _, dev := range devs {
		if !expectedIDs[dev.ID] {
			t.Errorf("unexpected device ID: %s", dev.ID)
		}
	}
}

func TestBuildVirtualDevices_AllUnique(t *testing.T) {
	physicalIDs := []string{"gpu-a", "gpu-b", "gpu-c"}
	devs := buildVirtualDevices(physicalIDs, 3)

	if len(devs) != 9 {
		t.Fatalf("expected 9 devices, got %d", len(devs))
	}

	seen := make(map[string]bool)
	for _, dev := range devs {
		if seen[dev.ID] {
			t.Errorf("duplicate device ID: %s", dev.ID)
		}
		seen[dev.ID] = true
	}
}

func TestBuildVirtualDevices_Empty(t *testing.T) {
	devs := buildVirtualDevices([]string{}, 4)
	if len(devs) != 0 {
		t.Fatalf("expected 0 devices for empty input, got %d", len(devs))
	}
}

func TestResolvePhysicalID_WithSliceSuffix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"0000:03:00.0-slice-0", "0000:03:00.0"},
		{"0000:03:00.0-slice-2", "0000:03:00.0"},
		{"0000:04:00.0-slice-99", "0000:04:00.0"},
		{"amdgpu_xcp_30-slice-1", "amdgpu_xcp_30"},
	}

	for _, tc := range tests {
		result := resolvePhysicalID(tc.input)
		if result != tc.expected {
			t.Errorf("resolvePhysicalID(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestResolvePhysicalID_WithoutSliceSuffix(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"0000:03:00.0"},
		{"amdgpu_xcp_30"},
		{"some-device-id"},
		{""},
	}

	for _, tc := range tests {
		result := resolvePhysicalID(tc.input)
		if result != tc.input {
			t.Errorf("resolvePhysicalID(%q) = %q, want %q (no-op)", tc.input, result, tc.input)
		}
	}
}

func TestResolvePhysicalID_RoundTrip(t *testing.T) {
	// Verify that resolvePhysicalID correctly undoes buildVirtualDevices naming
	physicalIDs := []string{"0000:03:00.0", "0000:04:00.0"}
	devs := buildVirtualDevices(physicalIDs, 3)

	for _, dev := range devs {
		resolved := resolvePhysicalID(dev.ID)
		found := false
		for _, pid := range physicalIDs {
			if resolved == pid {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("resolvePhysicalID(%q) = %q, not in original physical IDs", dev.ID, resolved)
		}
	}
}
