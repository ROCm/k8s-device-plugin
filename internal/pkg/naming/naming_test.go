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
	"testing"
)

func device(compute, memory string) map[string]interface{} {
	return map[string]interface{}{
		"computePartitionType": compute,
		"memoryPartitionType":  memory,
	}
}

func TestParseStrategy(t *testing.T) {
	for _, in := range []string{"single", "mixed"} {
		if _, err := ParseStrategy(in); err != nil {
			t.Errorf("ParseStrategy(%q) returned error: %v", in, err)
		}
	}
	if _, err := ParseStrategy("partition"); err == nil {
		t.Error("ParseStrategy(\"partition\") should have returned an error")
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResourceName(tt.device, tt.strategy); got != tt.want {
				t.Errorf("ResourceName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResourceNameToleratesMissingKeys(t *testing.T) {
	for _, strategy := range []Strategy{StrategySingle, StrategyMixed} {
		if got := ResourceName(map[string]interface{}{}, strategy); got != DefaultResource {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResourceList(tt.devices, tt.strategy)
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

	resources, err := ResourceList(devices, StrategyMixed)
	if err != nil {
		t.Fatalf("ResourceList() returned error: %v", err)
	}

	advertised := map[string]struct{}{}
	for _, r := range resources {
		advertised[r] = struct{}{}
	}

	for id, d := range devices {
		name := ResourceName(d, StrategyMixed)
		if _, ok := advertised[name]; !ok {
			t.Errorf("device %s maps to %q, which is not advertised", id, name)
		}
	}
}
