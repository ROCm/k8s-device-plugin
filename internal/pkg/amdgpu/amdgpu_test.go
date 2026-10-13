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

package amdgpu

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	FatalOnDriverUnavailable = false
	os.Exit(m.Run())
}

func hasAMDGPU(t *testing.T) bool {
	devices := GetAMDGPUs()

	if len(devices) <= 0 {
		return false
	}
	return true
}

func TestFirmwareVersionConsistent(t *testing.T) {
	if !hasAMDGPU(t) {
		t.Skip("Skipping test, no AMD GPU found.")
	}

	devices := GetAMDGPUs()

	for pci, dev := range devices {
		card := fmt.Sprintf("card%d", dev["card"])
		t.Logf("%s, %s", pci, card)

		//debugfs path/interface may not be stable
		debugFSfeatVersion, debugFSfwVersion :=
			parseDebugFSFirmwareInfo("/sys/kernel/debug/dri/" + card[4:] + "/amdgpu_firmware_info")
		featVersion, fwVersion, err := GetFirmwareVersions(card)
		if err != nil {
			t.Errorf("Fail to get firmware version %s", err.Error())
		}

		for k := range featVersion {
			if featVersion[k] != debugFSfeatVersion[k] {
				t.Errorf("%s feature version not consistent: ioctl: %d, debugfs: %d",
					k, featVersion[k], debugFSfeatVersion[k])
			}
			if fwVersion[k] != debugFSfwVersion[k] {
				t.Errorf("%s firmware version not consistent: ioctl: %x, debugfs: %x",
					k, fwVersion[k], debugFSfwVersion[k])
			}
		}
	}
}

func TestAMDGPUcountConsistent(t *testing.T) {
	if !hasAMDGPU(t) {
		t.Skip("Skipping test, no AMD GPU found.")
	}

	devices := GetAMDGPUs()

	matches, _ := filepath.Glob("/sys/class/drm/card[0-9]*/device/vendor")

	count := 0
	for _, vidPath := range matches {
		t.Log(vidPath)
		b, err := ioutil.ReadFile(vidPath)
		vid := string(b)

		// AMD vendor ID is 0x1002
		if err == nil && "0x1002" == strings.TrimSpace(vid) {
			count++
		} else {
			t.Log(vid)
		}

	}

	if count != len(devices) {
		t.Errorf("AMD GPU counts differ: /sys/module/amdgpu: %d, /sys/class/drm: %d", len(devices), count)
	}

}

func TestHasAMDGPU(t *testing.T) {
	if !hasAMDGPU(t) {
		t.Skip("Skipping test, no AMD GPU found.")
	}
}

func TestDevFunctional(t *testing.T) {
	if !hasAMDGPU(t) {
		t.Skip("Skipping test, no AMD GPU found.")
	}

	devices := GetAMDGPUs()

	for _, dev := range devices {
		card := fmt.Sprintf("card%d", dev["card"])

		ret := DevFunctional(card)
		t.Logf("%s functional: %t", card, ret)
	}
}

func TestParseTopologyProperties(t *testing.T) {
	var v int64
	var e error
	var re *regexp.Regexp
	var path string

	re = regexp.MustCompile(`size_in_bytes\s(\d+)`)
	path = "../../../testdata/topology-parsing/topology/nodes/1/mem_banks/0/properties"
	v, _ = ParseTopologyProperties(path, re)
	if v != 17163091968 {
		t.Errorf("Error parsing %s for `%s`: expect %d", path, re.String(), 17163091968)
	}

	re = regexp.MustCompile(`flags\s(\d+)`)
	path = "../../../testdata/topology-parsing/topology/nodes/1/mem_banks/0/properties"
	v, _ = ParseTopologyProperties(path, re)
	if v != 0 {
		t.Errorf("Error parsing %s for `%s`: expect %d", path, re.String(), 0)
	}

	re = regexp.MustCompile(`simd_count\s(\d+)`)
	path = "../../../testdata/topology-parsing/topology/nodes/2/properties"
	v, _ = ParseTopologyProperties(path, re)
	if v != 256 {
		t.Errorf("Error parsing %s for `%s`: expect %d", path, re.String(), 256)
	}

	re = regexp.MustCompile(`simd_id_base\s(\d+)`)
	path = "../../../testdata/topology-parsing/topology/nodes/2/properties"
	v, _ = ParseTopologyProperties(path, re)
	if v != 2147487744 {
		t.Errorf("Error parsing %s for `%s`: expect %d", path, re.String(), 2147487744)
	}

	re = regexp.MustCompile(`asdf\s(\d+)`)
	path = "../../../testdata/topology-parsing/topology/nodes/2/properties"
	_, e = ParseTopologyProperties(path, re)
	if e == nil {
		t.Errorf("Error parsing %s for `%s`: expect error", path, re.String())
	}

}

func TestParseDebugFSFirmwareInfo(t *testing.T) {
	expFeat := map[string]uint32{
		"VCE":   0,
		"UVD":   0,
		"MC":    0,
		"ME":    35,
		"PFP":   35,
		"CE":    35,
		"RLC":   0,
		"MEC":   33,
		"MEC2":  33,
		"SOS":   0,
		"ASD":   0,
		"SMC":   0,
		"SDMA0": 40,
		"SDMA1": 40,
	}

	expFw := map[string]uint32{
		"VCE":   0x352d0400,
		"UVD":   0x01571100,
		"MC":    0x00000000,
		"ME":    0x00000094,
		"PFP":   0x000000a4,
		"CE":    0x0000004a,
		"RLC":   0x00000058,
		"MEC":   0x00000160,
		"MEC2":  0x00000160,
		"SOS":   0x00161a92,
		"ASD":   0x0016129a,
		"SMC":   0x001c2800,
		"SDMA0": 0x00000197,
		"SDMA1": 0x00000197,
	}

	feat, fw := parseDebugFSFirmwareInfo("../../../testdata/debugfs-parsing/amdgpu_firmware_info")

	for k := range expFeat {
		val, ok := feat[k]
		if !ok || val != expFeat[k] {
			t.Errorf("Error parsing feature version for %s: expect %d", k, expFeat[k])
		}
	}

	for k := range expFw {
		val, ok := fw[k]
		if !ok || val != expFw[k] {
			t.Errorf("Error parsing firmware version for %s: expect %#08x", k, expFw[k])
		}
	}
	if len(feat) != len(expFeat) || len(fw) != len(expFw) {
		t.Errorf("Incorrect parsing of amdgpu firmware info from debugfs")
	}
}

func TestRenderDevIdsFromTopology(t *testing.T) {
	renderDevIds := GetDevIdsFromTopology("../../../testdata/topology-parsing-mi308")

	expDevIds := map[int]string{
		128: "0000:0a:00:0",
		129: "0000:0a:00:0",
		130: "0000:0a:00:0",
		131: "0000:0a:00:0",
		136: "0000:80:00:0",
		137: "0000:80:00:0",
		138: "0000:80:00:0",
		139: "0000:80:00:0",
		144: "0000:a4:00:0",
		145: "0000:a4:00:0",
		146: "0000:a4:00:0",
		147: "0000:a4:00:0",
		152: "0000:c8:00:0",
		153: "0000:c8:00:0",
		154: "0000:c8:00:0",
		155: "0000:c8:00:0",
		160: "0001:0b:00:0",
		161: "0001:0b:00:0",
		162: "0001:0b:00:0",
		163: "0001:0b:00:0",
		168: "0001:81:00:0",
		169: "0001:81:00:0",
		170: "0001:81:00:0",
		171: "0001:81:00:0",
		176: "0001:a5:00:0",
		177: "0001:a5:00:0",
		178: "0001:a5:00:0",
		179: "0001:a5:00:0",
		184: "0001:c9:00:0",
		185: "0001:c9:00:0",
		186: "0001:c9:00:0",
		187: "0001:c9:00:0"}
	if !reflect.DeepEqual(renderDevIds, expDevIds) {
		val, _ := json.MarshalIndent(renderDevIds, "", "  ")
		exp, _ := json.MarshalIndent(expDevIds, "", "  ")

		t.Errorf("RenderNode set was incorrect")
		t.Errorf("Got: %s", val)
		t.Errorf("Want: %s", exp)
	}
}

func TestResolveDeviceIdentity(t *testing.T) {
	tests := []struct {
		name            string
		devPaths        []string
		renderDevIDs    map[int]string
		renderNodeIDs   map[int]int
		want            deviceIdentity
		wantErrContains string
	}{
		{
			name:          "complete identity",
			devPaths:      []string{"/sys/devices/card0", "/sys/devices/renderD128"},
			renderDevIDs:  map[int]string{128: "0000:01:00:0"},
			renderNodeIDs: map[int]int{128: 1},
			want: deviceIdentity{
				card:    0,
				renderD: 128,
				devID:   "0000:01:00:0",
				nodeID:  1,
			},
		},
		{
			name:          "KFD node zero is valid",
			devPaths:      []string{"/sys/devices/card0", "/sys/devices/renderD128"},
			renderDevIDs:  map[int]string{128: "0000:01:00:0"},
			renderNodeIDs: map[int]int{128: 0},
			want: deviceIdentity{
				card:    0,
				renderD: 128,
				devID:   "0000:01:00:0",
				nodeID:  0,
			},
		},
		{
			name:            "missing card",
			devPaths:        []string{"/sys/devices/renderD128"},
			renderDevIDs:    map[int]string{128: "0000:01:00:0"},
			renderNodeIDs:   map[int]int{128: 1},
			wantErrContains: "incomplete DRM identity",
		},
		{
			name:            "missing render node",
			devPaths:        []string{"/sys/devices/card0"},
			renderDevIDs:    map[int]string{128: "0000:01:00:0"},
			renderNodeIDs:   map[int]int{128: 1},
			wantErrContains: "incomplete DRM identity",
		},
		{
			name:            "missing KFD device identity",
			devPaths:        []string{"/sys/devices/card1", "/sys/devices/renderD129"},
			renderDevIDs:    map[int]string{},
			renderNodeIDs:   map[int]int{129: 2},
			wantErrContains: "no KFD device identity",
		},
		{
			name:            "empty KFD device identity",
			devPaths:        []string{"/sys/devices/card1", "/sys/devices/renderD129"},
			renderDevIDs:    map[int]string{129: ""},
			renderNodeIDs:   map[int]int{129: 2},
			wantErrContains: "no KFD device identity",
		},
		{
			name:            "missing KFD node ID",
			devPaths:        []string{"/sys/devices/card1", "/sys/devices/renderD129"},
			renderDevIDs:    map[int]string{129: "0000:02:00:0"},
			renderNodeIDs:   map[int]int{},
			wantErrContains: "no KFD node ID",
		},
		{
			name: "short and unrelated entries do not panic",
			devPaths: []string{
				"/sys/devices/c",
				"/sys/devices/controlD64",
				"/sys/devices/renderD128",
			},
			renderDevIDs:    map[int]string{128: "0000:01:00:0"},
			renderNodeIDs:   map[int]int{128: 1},
			wantErrContains: "incomplete DRM identity",
		},
		{
			name:            "card index that does not fit an int",
			devPaths:        []string{"/sys/devices/card99999999999999999999", "/sys/devices/renderD128"},
			renderDevIDs:    map[int]string{128: "0000:01:00:0"},
			renderNodeIDs:   map[int]int{128: 1},
			wantErrContains: "parse DRM card entry",
		},
		{
			name:            "render index that does not fit an int",
			devPaths:        []string{"/sys/devices/card0", "/sys/devices/renderD99999999999999999999"},
			renderDevIDs:    map[int]string{128: "0000:01:00:0"},
			renderNodeIDs:   map[int]int{128: 1},
			wantErrContains: "parse DRM render entry",
		},
		{
			name: "connector entry is not a card entry",
			devPaths: []string{
				"/sys/devices/card0-DP-1",
				"/sys/devices/renderD128",
			},
			renderDevIDs:    map[int]string{128: "0000:01:00:0"},
			renderNodeIDs:   map[int]int{128: 1},
			wantErrContains: "incomplete DRM identity",
		},
		{
			name: "render entry with a trailing suffix is not a render entry",
			devPaths: []string{
				"/sys/devices/card0",
				"/sys/devices/renderD128x",
			},
			renderDevIDs:    map[int]string{128: "0000:01:00:0"},
			renderNodeIDs:   map[int]int{128: 1},
			wantErrContains: "incomplete DRM identity",
		},
		{
			name: "conflicting cards",
			devPaths: []string{
				"/sys/devices/card0",
				"/sys/devices/card1",
				"/sys/devices/renderD128",
			},
			renderDevIDs:    map[int]string{128: "0000:01:00:0"},
			renderNodeIDs:   map[int]int{128: 1},
			wantErrContains: "conflicting DRM card entries",
		},
		{
			name: "conflicting render nodes",
			devPaths: []string{
				"/sys/devices/card0",
				"/sys/devices/renderD128",
				"/sys/devices/renderD129",
			},
			renderDevIDs: map[int]string{
				128: "0000:01:00:0",
				129: "0000:02:00:0",
			},
			renderNodeIDs: map[int]int{
				128: 1,
				129: 2,
			},
			wantErrContains: "conflicting DRM render entries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveDeviceIdentity(
				tt.devPaths,
				tt.renderDevIDs,
				tt.renderNodeIDs,
			)

			if tt.wantErrContains != "" {
				if err == nil {
					t.Fatalf(
						"resolveDeviceIdentity() error = nil, want %q",
						tt.wantErrContains,
					)
				}
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf(
						"resolveDeviceIdentity() error = %q, want substring %q",
						err,
						tt.wantErrContains,
					)
				}
				return
			}

			if err != nil {
				t.Fatalf("resolveDeviceIdentity() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf(
					"resolveDeviceIdentity() = %#v, want %#v",
					got,
					tt.want,
				)
			}
		})
	}
}

func TestResolveDeviceIdentityDoesNotReusePreviousDevice(t *testing.T) {
	renderDevIDs := map[int]string{
		128: "0000:01:00:0",
	}
	renderNodeIDs := map[int]int{
		128: 1,
	}

	first, err := resolveDeviceIdentity(
		[]string{
			"/sys/devices/card0",
			"/sys/devices/renderD128",
		},
		renderDevIDs,
		renderNodeIDs,
	)
	if err != nil {
		t.Fatalf("resolve first device: %v", err)
	}
	if first.devID != "0000:01:00:0" {
		t.Fatalf(
			"first.devID = %q, want %q",
			first.devID,
			"0000:01:00:0",
		)
	}

	_, err = resolveDeviceIdentity(
		[]string{
			"/sys/devices/card1",
			"/sys/devices/renderD129",
		},
		renderDevIDs,
		renderNodeIDs,
	)
	if err == nil {
		t.Fatal(
			"second device unexpectedly reused the first device's KFD identity",
		)
	}
}

// TestDevIdsFromTopologyOmitsPCIFunction records that the KFD-derived device
// identity is built from the bus and device numbers only, so two functions of
// the same PCI device share one. GetAMDGPUs relies on this when it decides
// which physical GPU a partition belongs to.
func TestDevIdsFromTopologyOmitsPCIFunction(t *testing.T) {
	topoRoot := t.TempDir()

	// the low three bits of location_id hold the PCI function
	for i, locationId := range []int{0x100, 0x101} {
		nodeDir := filepath.Join(topoRoot, "topology", "nodes", fmt.Sprint(i+2))
		if err := os.MkdirAll(nodeDir, 0o755); err != nil {
			t.Fatalf("Failed to create %s: %s", nodeDir, err)
		}
		properties := fmt.Sprintf("location_id %d\ndomain 0\ndrm_render_minor %d\n", locationId, 128+i*8)
		if err := os.WriteFile(filepath.Join(nodeDir, "properties"), []byte(properties), 0o644); err != nil {
			t.Fatalf("Failed to write properties: %s", err)
		}
	}

	renderDevIds := GetDevIdsFromTopology(topoRoot)

	if len(renderDevIds) != 2 {
		t.Fatalf("GetDevIdsFromTopology returned %d entries, expect 2", len(renderDevIds))
	}
	if renderDevIds[128] != renderDevIds[136] {
		t.Errorf("function 0 resolved to %q and function 1 to %q, expect the same device identity",
			renderDevIds[128], renderDevIds[136])
	}
}

func TestRecordParentGPU(t *testing.T) {
	parents := make(map[string]map[string]interface{})
	ambiguous := make(map[string]struct{})

	first := map[string]interface{}{"card": 0}
	second := map[string]interface{}{"card": 1}
	third := map[string]interface{}{"card": 2}
	other := map[string]interface{}{"card": 3}

	if !recordParentGPU(parents, ambiguous, "0000:01:00:0", first) {
		t.Fatal("recordParentGPU rejected the first GPU claiming an identity")
	}
	if !reflect.DeepEqual(parents["0000:01:00:0"], first) {
		t.Errorf("0000:01:00:0 resolved to %v, expect the first GPU", parents["0000:01:00:0"])
	}

	// a second function of the same PCI device shares the identity
	if recordParentGPU(parents, ambiguous, "0000:01:00:0", second) {
		t.Error("recordParentGPU accepted a second GPU claiming the same identity")
	}
	if _, exists := parents["0000:01:00:0"]; exists {
		t.Error("an identity claimed twice must not name a parent")
	}

	// a third claim must not resurrect it
	if recordParentGPU(parents, ambiguous, "0000:01:00:0", third) {
		t.Error("recordParentGPU accepted a third GPU claiming the same identity")
	}
	if _, exists := parents["0000:01:00:0"]; exists {
		t.Error("a third claim resurrected an ambiguous identity")
	}

	// an unrelated identity is unaffected
	if !recordParentGPU(parents, ambiguous, "0000:02:00:0", other) {
		t.Error("recordParentGPU rejected an unambiguous identity")
	}
	if len(parents) != 1 {
		t.Errorf("parent inventory holds %d entries, expect only the unambiguous one", len(parents))
	}
}

func TestPartitionMetadataFromParent(t *testing.T) {
	tests := []struct {
		name            string
		parent          map[string]interface{}
		wantCompute     string
		wantMemory      string
		wantNuma        int
		wantErrContains string
	}{
		{
			name:        "complete metadata",
			parent:      map[string]interface{}{"computePartitionType": "cpx", "memoryPartitionType": "nps4", "numaNode": 1},
			wantCompute: "cpx", wantMemory: "nps4", wantNuma: 1,
		},
		{
			name:        "NUMA node zero is valid",
			parent:      map[string]interface{}{"computePartitionType": "spx", "memoryPartitionType": "nps1", "numaNode": 0},
			wantCompute: "spx", wantMemory: "nps1", wantNuma: 0,
		},
		{
			name:            "compute partition type missing from the map",
			parent:          map[string]interface{}{"memoryPartitionType": "nps1", "numaNode": 0},
			wantErrContains: "malformed partition metadata",
		},
		{
			name:            "numa node of the wrong type",
			parent:          map[string]interface{}{"computePartitionType": "spx", "memoryPartitionType": "nps1", "numaNode": "0"},
			wantErrContains: "malformed partition metadata",
		},
		{
			name:            "empty compute partition type",
			parent:          map[string]interface{}{"computePartitionType": "", "memoryPartitionType": "nps1", "numaNode": 0},
			wantErrContains: "no partition type",
		},
		{
			name:            "empty memory partition type",
			parent:          map[string]interface{}{"computePartitionType": "spx", "memoryPartitionType": "", "numaNode": 0},
			wantErrContains: "no partition type",
		},
		{
			name:            "no NUMA affinity",
			parent:          map[string]interface{}{"computePartitionType": "spx", "memoryPartitionType": "nps1", "numaNode": -1},
			wantErrContains: "no NUMA node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compute, memory, numa, err := partitionMetadataFromParent(tt.parent)

			if tt.wantErrContains != "" {
				if err == nil {
					t.Fatalf("partitionMetadataFromParent() error = nil, want %q", tt.wantErrContains)
				}
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("partitionMetadataFromParent() error = %q, want substring %q", err, tt.wantErrContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("partitionMetadataFromParent() error = %v", err)
			}
			if compute != tt.wantCompute || memory != tt.wantMemory || numa != tt.wantNuma {
				t.Errorf("partitionMetadataFromParent() = %q, %q, %d, want %q, %q, %d",
					compute, memory, numa, tt.wantCompute, tt.wantMemory, tt.wantNuma)
			}
		})
	}
}
