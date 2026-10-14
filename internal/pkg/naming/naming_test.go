/**
 * Copyright 2026 Advanced Micro Devices, Inc.  All rights reserved.
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

package naming

import (
	"reflect"
	"strings"
	"testing"
)

func device(compute, memory string) map[string]interface{} {
	return map[string]interface{}{
		"computePartitionType": compute,
		"memoryPartitionType":  memory,
	}
}

func modelDevice(compute, memory, deviceID, revisionID string) map[string]interface{} {
	d := device(compute, memory)
	d["deviceID"] = deviceID
	d["revisionID"] = revisionID
	return d
}

// testModels stands in for a parsed amdgpu.ids table.
var testModels = ProductIndex{
	ProductID{DeviceID: "74A1", RevisionID: "00"}: "gpu-mi300x",
	ProductID{DeviceID: "740F", RevisionID: "02"}: "gpu-mi210",
}

func TestParseStrategy(t *testing.T) {
	for _, in := range []string{"single", "mixed", "product"} {
		if _, err := ParseStrategy(in); err != nil {
			t.Errorf("ParseStrategy(%q) returned error: %v", in, err)
		}
	}
	if _, err := ParseStrategy("partition"); err == nil {
		t.Error("ParseStrategy(\"partition\") should have returned an error")
	}
}

func TestSanitizeResourceName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"instinct", "AMD Instinct MI300X OAM", "amd-instinct-mi300x-oam"},
		{"slash collapses", "AMD Instinct MI250X / MI250", "amd-instinct-mi250x-mi250"},
		{"plus is replaced", "Ryzen AI Max+ 395", "ryzen-ai-max-395"},
		{"parens", "AMD Radeon RX 7900 XTX (gfx1100)", "amd-radeon-rx-7900-xtx-gfx1100"},
		{"already clean", "mi210", "mi210"},
		{"surrounding junk", "  --MI300X--  ", "mi300x"},
		{"empty", "", ""},
		{"only separators", " + / ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeResourceName(tt.in); got != tt.want {
				t.Errorf("SanitizeResourceName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSanitizeResourceNameTruncates(t *testing.T) {
	got := SanitizeResourceName(strings.Repeat("ab ", 40))
	if len(got) > maxResourceNameLen {
		t.Errorf("SanitizeResourceName returned %d chars, want <= %d", len(got), maxResourceNameLen)
	}
	if strings.HasSuffix(got, "-") || strings.HasPrefix(got, "-") {
		t.Errorf("SanitizeResourceName returned %q, want no leading or trailing separator", got)
	}
}

func TestResourceName(t *testing.T) {
	tests := []struct {
		name     string
		device   map[string]interface{}
		strategy Strategy
		want     string
	}{
		{"single ignores partition", device("cpx", "nps4"), StrategySingle, "gpu"},
		{"mixed uses partition", device("cpx", "nps4"), StrategyMixed, "cpx_nps4"},
		{"mixed falls back when unpartitionable", device("", ""), StrategyMixed, "gpu"},
		{"mixed falls back on half a style", device("spx", ""), StrategyMixed, "gpu"},
		{"product uses the marketing name", modelDevice("spx", "nps1", "74A1", "00"), StrategyProduct, "gpu-mi300x-spx-nps1"},
		{"product accepts the sysfs 0x spelling", modelDevice("spx", "nps1", "0x74a1", "0x00"), StrategyProduct, "gpu-mi300x-spx-nps1"},
		// Dropping the style here would pool a CPX partition with a whole GPU,
		// which mixed keeps apart.
		{"product keeps the style when absent from amdgpu.ids", modelDevice("cpx", "nps4", "74FF", "00"), StrategyProduct, "gpu-cpx-nps4"},
		{"product falls back to gpu when unpartitioned and absent", modelDevice("", "", "74FF", "00"), StrategyProduct, "gpu"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResourceName(tt.device, tt.strategy, testModels); got != tt.want {
				t.Errorf("ResourceName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResourceNameToleratesMissingKeys(t *testing.T) {
	for _, strategy := range []Strategy{StrategySingle, StrategyMixed, StrategyProduct} {
		if got := ResourceName(map[string]interface{}{}, strategy, testModels); got != DefaultResource {
			t.Errorf("ResourceName(empty, %q) = %q, want %q", strategy, got, DefaultResource)
		}
	}
}

func TestResourceList(t *testing.T) {
	tests := []struct {
		name     string
		devices  map[string]map[string]interface{}
		strategy Strategy
		want     []string
		wantErr  bool
	}{
		{
			name:     "no devices",
			devices:  map[string]map[string]interface{}{},
			strategy: StrategySingle,
			want:     nil,
		},
		{
			name: "single on a homogeneous node",
			devices: map[string]map[string]interface{}{
				"a": device("spx", "nps1"),
				"b": device("spx", "nps1"),
			},
			strategy: StrategySingle,
			want:     []string{"gpu"},
		},
		{
			name: "single rejects mixed partition styles",
			devices: map[string]map[string]interface{}{
				"a": device("spx", "nps1"),
				"b": device("cpx", "nps4"),
			},
			strategy: StrategySingle,
			wantErr:  true,
		},
		{
			name: "mixed reports each partition style",
			devices: map[string]map[string]interface{}{
				"a": device("spx", "nps1"),
				"b": device("cpx", "nps4"),
				"c": device("cpx", "nps4"),
			},
			strategy: StrategyMixed,
			want:     []string{"cpx_nps4", "spx_nps1"},
		},
		{
			name: "mixed reports unpartitionable GPUs as gpu",
			devices: map[string]map[string]interface{}{
				"a": device("", ""),
				"b": device("", ""),
			},
			strategy: StrategyMixed,
			want:     []string{"gpu"},
		},
		{
			name: "mixed reports an unpartitionable GPU alongside partitioned ones",
			devices: map[string]map[string]interface{}{
				"a": device("spx", "nps1"),
				"b": device("cpx", "nps4"),
				"c": device("", ""),
			},
			strategy: StrategyMixed,
			want:     []string{"cpx_nps4", "gpu", "spx_nps1"},
		},
		{
			name: "product reports each marketing name",
			devices: map[string]map[string]interface{}{
				"a": modelDevice("spx", "nps1", "74A1", "00"),
				"b": modelDevice("", "", "740F", "02"),
				"c": modelDevice("", "", "740F", "02"),
			},
			strategy: StrategyProduct,
			want:     []string{"gpu-mi210", "gpu-mi300x-spx-nps1"},
		},
		{
			name: "product groups a partitioned GPU under its parent",
			devices: map[string]map[string]interface{}{
				"a": modelDevice("cpx", "nps4", "74A1", "00"),
				"b": modelDevice("cpx", "nps4", "74A1", "00"),
				"c": modelDevice("cpx", "nps4", "74A1", "00"),
			},
			strategy: StrategyProduct,
			want:     []string{"gpu-mi300x-cpx-nps4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResourceList(tt.devices, tt.strategy, testModels)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ResourceList() returned no error, want one")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResourceList() returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ResourceList() = %v, want %v", got, tt.want)
			}
		})
	}
}

// A device whose ResourceName is not in ResourceList is served by no plugin.
func TestResourceListAgreesWithResourceName(t *testing.T) {
	devices := map[string]map[string]interface{}{
		"a": device("spx", "nps1"),
		"b": device("cpx", "nps4"),
		"c": device("", ""),
	}

	resources, err := ResourceList(devices, StrategyMixed, testModels)
	if err != nil {
		t.Fatalf("ResourceList() returned error: %v", err)
	}

	advertised := map[string]struct{}{}
	for _, r := range resources {
		advertised[r] = struct{}{}
	}

	for id, d := range devices {
		name := ResourceName(d, StrategyMixed, testModels)
		if _, ok := advertised[name]; !ok {
			t.Errorf("device %s maps to %q, which is not advertised", id, name)
		}
	}
}
