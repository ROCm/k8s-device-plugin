/**
# Copyright 2025 Advanced Micro Devices, Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the \"License\");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an \"AS IS\" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package allocator

import (
	"fmt"
	"sort"
	"testing"
)

func TestBestPolicyAllocator(t *testing.T) {
	topos := []testInfo{
		{
			devCount:             4,
			partitionCountPerDev: 8,
			numanodeCount:        2,
			startNodeId:          2,
			endNodeId:            33,
			topoFolderPath:       "../../../testdata/topology-parsing-mi308/topology/nodes",
		},
		{
			devCount:             8,
			partitionCountPerDev: 1,
			numanodeCount:        2,
			startNodeId:          2,
			endNodeId:            9,
			topoFolderPath:       "../../../testdata/topo-mi210-xgmi-pcie/nodes",
		},
		{
			devCount:             8,
			partitionCountPerDev: 8,
			numanodeCount:        2,
			startNodeId:          2,
			endNodeId:            64,
			topoFolderPath:       "../../../testdata/topo-mi300-cpx/topology/nodes",
		},
	}

	testcases := [][]testInfo{
		[]testInfo{
			{
				description: "Allocate 1 device",
				size:        1,
				required:    nil,
			},
			{
				description: "Allocate 3 devices",
				size:        3,
				required:    nil,
			},
			{
				description: "Allocate 12 devices",
				size:        12,
				required:    nil,
			},
		},
		[]testInfo{
			{
				description: "Allocate 1 device",
				size:        1,
				required:    nil,
				expectedIds: []string{"test1"},
			},
			{
				description: "Allocate 3 devices",
				size:        3,
				required:    nil,
				expectedIds: []string{"test1", "test2", "test3"},
			},
			{
				description: "Allocate 5 devices",
				size:        5,
				required:    nil,
				expectedIds: []string{"test1", "test2", "test3", "test4", "test5"},
			},
			{
				description: "Allocate 3 - same numa",
				size:        3,
				available:   []string{"test3", "test4", "test5", "test6", "test7", "test8"},
				required:    nil,
				expectedIds: []string{"test5", "test6", "test7"},
			},
		},
		[]testInfo{
			{
				description: "Allocate 1 device",
				size:        1,
				required:    nil,
				expectedIds: []string{"test8"},
			},
			{
				description: "Allocate 3 devices",
				size:        3,
				required:    nil,
				expectedIds: []string{"test8", "amdgpu_xcp_57", "amdgpu_xcp_58"},
			},
			{
				description: "Allocate 5 devices",
				size:        5,
				required:    nil,
				expectedIds: []string{"test8", "amdgpu_xcp_57", "amdgpu_xcp_58", "amdgpu_xcp_59", "amdgpu_xcp_60"},
			},
			{
				description: "Allocate 3 - same numa",
				size:        3,
				available:   []string{"test3", "test4", "test5", "test6", "test7", "test8"},
				required:    nil,
				expectedIds: []string{"test5", "test6", "test7"},
			},
			{
				description: "Allocate 3 - with required and same numa",
				size:        3,
				available:   []string{"test3", "test4", "test5", "test6", "test7", "test8"},
				required:    []string{"test5"},
				expectedIds: []string{"test5", "test6", "test7"},
			},
			{
				description: "Allocate 30 devices",
				size:        30,
			},
			{
				description: "Allocate 8 devices",
				size:        8,
				expectedIds: []string{"test1", "amdgpu_xcp_1", "amdgpu_xcp_2", "amdgpu_xcp_3", "amdgpu_xcp_4", "amdgpu_xcp_5", "amdgpu_xcp_6", "amdgpu_xcp_7"},
			},
			{
				description: "Allocate 7 devices",
				size:        7,
				required:    nil,
				expectedIds: []string{"test8", "amdgpu_xcp_57", "amdgpu_xcp_58", "amdgpu_xcp_59", "amdgpu_xcp_60", "amdgpu_xcp_61", "amdgpu_xcp_62"},
			},
			{
				description: "Allocate 4 devices",
				size:        4,
				required:    nil,
				filtered:    []string{"test8", "amdgpu_xcp_57", "amdgpu_xcp_58"},
				expectedIds: []string{"amdgpu_xcp_59", "amdgpu_xcp_60", "amdgpu_xcp_61", "amdgpu_xcp_62"},
			},
			{
				description: "Allocate 10 devices",
				size:        10,
				required:    nil,
				filtered:    []string{"test1", "test2", "test3", "test4", "test8", "amdgpu_xcp_57"},
				expectedIds: []string{"test5", "amdgpu_xcp_33", "amdgpu_xcp_34", "amdgpu_xcp_35", "amdgpu_xcp_36", "amdgpu_xcp_37", "amdgpu_xcp_38", "amdgpu_xcp_39", "amdgpu_xcp_58", "amdgpu_xcp_59"},
			},
		},
	}

	for idx, topo := range topos {
		devices := topo.getTestDevices()
		allAvailableIds := make([]string, 0)
		for _, d := range devices {
			allAvailableIds = append(allAvailableIds, d.Id)
		}
		a := NewBestEffortPolicy()
		a.Init(devices, topo.topoFolderPath)
		t.Logf("-------BEGIN tests for Topology %d-------", idx+1)
		for _, tc := range testcases[idx] {
			t.Logf("-----Starting testcase: %s", tc.description)
			var av, req []string
			if len(tc.available) > 0 {
				av = tc.available
			} else {
				av = allAvailableIds
			}
			if len(tc.filtered) > 0 {
				av = topo.getFilteredDeviceIds(av, tc.filtered)
			}
			if len(tc.required) > 0 {
				req = tc.required
			}
			result, err := a.Allocate(av, req, tc.size)
			tc.result = "PASS"
			if err != nil {
				t.Errorf("expected Allocate method to pass. But failed with error %v", err)
				tc.result = "FAIL"
			}
			if len(result) != tc.size {
				t.Errorf("expected result to have %d devices. But got %d", tc.size, len(result))
				tc.result = "FAIL"
			}
			sort.Slice(result, func(i, j int) bool {
				return result[i] < result[j]
			})
			fmt.Println(result)
			if len(tc.expectedIds) > 0 {
				sort.Slice(tc.expectedIds, func(i, j int) bool {
					return tc.expectedIds[i] < tc.expectedIds[j]
				})
				for i := 0; i < tc.size; i++ {
					if result[i] != tc.expectedIds[i] {
						t.Errorf("result set not as expected. Expected %v but got %v", tc.expectedIds, result)
						tc.result = "FAIL"
					}
				}
			}
			t.Logf("Result: %v", tc.result)
			t.Logf("-----End testcase: %s", tc.description)
		}
		t.Logf("-------END tests for Topology %d-------", idx+1)
	}
}
