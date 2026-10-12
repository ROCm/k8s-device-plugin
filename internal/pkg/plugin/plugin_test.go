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
	"reflect"
	"testing"

	"github.com/ROCm/k8s-device-plugin/internal/pkg/naming"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

func TestCountGPUDevFromTopology(t *testing.T) {
	count := countGPUDevFromTopology("../../../testdata/topology-parsing")

	expCount := 2
	if count != expCount {
		t.Errorf("Count was incorrect, got: %d, want: %d.", count, expCount)
	}
}

func gpu(numa int, compute, memory string) map[string]interface{} {
	return map[string]interface{}{
		"numaNode":             numa,
		"computePartitionType": compute,
		"memoryPartitionType":  memory,
	}
}

func bucketSizes(buckets map[string][]*pluginapi.Device) map[string]int {
	sizes := map[string]int{}
	for resource, devs := range buckets {
		sizes[resource] = len(devs)
	}
	return sizes
}

func TestBucketDevices(t *testing.T) {
	tests := []struct {
		name     string
		strategy naming.Strategy
		gpus     map[string]map[string]interface{}
		want     map[string]int
	}{
		{
			name:     "single serves the whole node as one resource",
			strategy: naming.StrategySingle,
			gpus: map[string]map[string]interface{}{
				"0000:01:00.0": gpu(0, "spx", "nps1"),
				"0000:02:00.0": gpu(1, "spx", "nps1"),
			},
			want: map[string]int{"gpu": 2},
		},
		{
			name:     "mixed separates partition styles",
			strategy: naming.StrategyMixed,
			gpus: map[string]map[string]interface{}{
				"0000:01:00.0": gpu(0, "spx", "nps1"),
				"0000:02:00.0": gpu(1, "cpx", "nps4"),
				"0000:03:00.0": gpu(1, "cpx", "nps4"),
			},
			want: map[string]int{"spx_nps1": 1, "cpx_nps4": 2},
		},
		{
			// This GPU used to be keyed into a bucket getResourceList never
			// advertised, so no plugin instance served it and it went unreported.
			name:     "mixed reports an unpartitionable GPU alongside partitioned ones",
			strategy: naming.StrategyMixed,
			gpus: map[string]map[string]interface{}{
				"0000:01:00.0": gpu(0, "cpx", "nps4"),
				"0000:02:00.0": gpu(1, "", ""),
			},
			want: map[string]int{"cpx_nps4": 1, "gpu": 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bucketSizes(bucketDevices(tt.gpus, tt.strategy))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("bucketDevices() = %v, want %v", got, tt.want)
			}
		})
	}
}
