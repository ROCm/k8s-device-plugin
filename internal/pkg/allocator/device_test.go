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
	"slices"
	"sort"
	"strconv"
	"testing"
	"time"
)

type testInfo struct {
	description           string
	devCount              int
	partitionCountPerDev  int
	numanodeCount         int
	startNodeId           int
	endNodeId             int
	available             []string
	filtered              []string
	required              []string
	size                  int
	expectedIds           []string
	expectedSubsetsLength int
	topoFolderPath        string
	result                string
}

func (ti *testInfo) getTestDevices() []*Device {
	var res []*Device
	nodeId := ti.startNodeId
	for i := 0; i < ti.devCount; i++ {
		numa := ti.devCount / ti.numanodeCount
		for j := 0; j < ti.partitionCountPerDev; j++ {
			//partitioned gpus have id starting with amdgpu_xcp
			id := fmt.Sprintf("amdgpu_xcp_%d", (i*8)+j)
			if j == 0 {
				id = fmt.Sprintf("test%d", i+1)
			}
			if nodeId > ti.endNodeId {
				break
			}
			res = append(res, &Device{
				Id:       id,
				NodeId:   nodeId,
				NumaNode: i / numa,
				DevId:    strconv.Itoa(i),
			})
			nodeId = nodeId + 1
		}
	}
	return res
}

func (ti *testInfo) getFilteredDeviceIds(available, filter []string) []string {
	var res []string
	for _, av := range available {
		idx := slices.Index(filter, av)
		if idx == -1 {
			res = append(res, av)
		}
	}
	return res
}

func TestPairWeightsCalculationEmptyDevices(t *testing.T) {
	folderPath := "../../../testdata/topology-parsing-mi308/topology/nodes"
	p2pWeights := make(map[int]map[int]int)
	var devices []*Device
	err := fetchAllPairWeights(devices, p2pWeights, folderPath)
	if err == nil {
		t.Errorf("fetchAllPairWeights call is expected to return error but got nil")
	}
}

func TestPairWeightsCalculation(t *testing.T) {
	p2pWeights := make(map[int]map[int]int)
	tinfo := testInfo{
		devCount:             4,
		partitionCountPerDev: 8,
		numanodeCount:        2,
		startNodeId:          2,
		endNodeId:            33,
		topoFolderPath:       "../../../testdata/topology-parsing-mi308/topology/nodes",
	}
	devices := tinfo.getTestDevices()
	err := fetchAllPairWeights(devices, p2pWeights, tinfo.topoFolderPath)
	if err != nil {
		t.Errorf("fetchAllPairWeights call failed. Error:%v", err)
	}
	if len(p2pWeights) != 31 {
		t.Errorf("expected p2pWeights length to be 31, but got %d\n", len(p2pWeights))
	}
}

func TestGroupPartitionsByDevId(t *testing.T) {
	tinfo := testInfo{
		devCount:             4,
		partitionCountPerDev: 8,
		numanodeCount:        2,
		startNodeId:          2,
		endNodeId:            33,
	}
	devices := tinfo.getTestDevices()
	devIdMap := groupPartitionsByDevId(devices)
	if len(devIdMap) != 4 {
		t.Errorf("groupPartitionsByDevId call failed. Expected map length to be 4 but got :%v", len(devIdMap))
	}
}

func TestGetSubsetsMethod(t *testing.T) {
	p2pWeights := make(map[int]map[int]int)
	tinfo := testInfo{
		devCount:             4,
		partitionCountPerDev: 8,
		numanodeCount:        2,
		startNodeId:          2,
		endNodeId:            33,
		topoFolderPath:       "../../../testdata/topology-parsing-mi308/topology/nodes",
	}
	devices := tinfo.getTestDevices()
	devIdMap := groupPartitionsByDevId(devices)
	err := fetchAllPairWeights(devices, p2pWeights, tinfo.topoFolderPath)
	if err != nil {
		t.Errorf("fetchAllPairWeights call failed. Error:%v", err)
	}

	testcases := []testInfo{
		{
			description:           "Get candidates with size 3",
			size:                  3,
			expectedSubsetsLength: 4,
		},
		{
			description:           "Get candidates with size 12",
			size:                  12,
			expectedSubsetsLength: 12,
		},
	}
	for _, tcase := range testcases {
		t.Logf("Starting testcase %s", tcase.description)
		tcase.result = "PASS"
		subsets, err := getCandidateDeviceSubsets(devIdMap, devices, devices, nil, tcase.size, p2pWeights)
		if err != nil {
			t.Errorf("expected getAllDeviceSubsets to pass. But got error %v", err)
			tcase.result = "FAIL"
		}
		if len(subsets) != tcase.expectedSubsetsLength {
			t.Errorf("expected subsets length to be %d but got %d", tcase.expectedSubsetsLength, len(subsets))
			tcase.result = "FAIL"
		}
		t.Logf("Result: %v", tcase.result)
		t.Logf("Ending Testcase %s", tcase.description)
	}
}

func (ti *testInfo) getSyntheticDevices(gpuCount, partitionsPerGPU int) []*Device {
	devs := make([]*Device, 0, gpuCount*partitionsPerGPU)
	nodeId := 2
	for gpu := 0; gpu < gpuCount; gpu++ {
		for partition := 0; partition < partitionsPerGPU; partition++ {
			id := fmt.Sprintf("amdgpu_xcp_%d", gpu*partitionsPerGPU+partition)
			if partition == 0 {
				id = fmt.Sprintf("gpu%d", gpu)
			}
			devs = append(devs, &Device{
				Id:       id,
				NodeId:   nodeId,
				NumaNode: gpu % 2,
				DevId:    strconv.Itoa(gpu),
			})
			nodeId++
		}
	}
	return devs
}

// TestCandidateSubsetsAreCombinations checks that a set of devices is offered
// as a candidate once, not once per order the enumeration can reach it in.
func TestCandidateSubsetsAreCombinations(t *testing.T) {
	tests := []struct {
		name             string
		gpuCount         int
		partitionsPerGPU int
		size             int
		expectCount      int
	}{
		// C(4,2) rather than P(4,2) = 12
		{name: "4 whole gpus, allocate 2", gpuCount: 4, partitionsPerGPU: 1, size: 2, expectCount: 6},
		// C(8,7) rather than P(8,7) = 40320
		{name: "8 gpus of 8 partitions, allocate 56", gpuCount: 8, partitionsPerGPU: 8, size: 56, expectCount: 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ti := &testInfo{}
			devs := ti.getSyntheticDevices(tt.gpuCount, tt.partitionsPerGPU)
			subsets, err := getCandidateDeviceSubsets(
				groupPartitionsByDevId(devs), devs, devs, nil, tt.size, map[int]map[int]int{})
			if err != nil {
				t.Fatalf("getCandidateDeviceSubsets failed: %s", err)
			}
			if len(subsets) != tt.expectCount {
				t.Errorf("got %d candidate subsets, expect %d", len(subsets), tt.expectCount)
			}

			seen := make(map[string]struct{}, len(subsets))
			for _, subset := range subsets {
				ids := make([]int, len(subset.Ids))
				copy(ids, subset.Ids)
				sort.Ints(ids)
				key := fmt.Sprint(ids)
				if _, duplicate := seen[key]; duplicate {
					t.Errorf("device set %v was offered more than once", ids)
				}
				seen[key] = struct{}{}
				if len(subset.Ids) != tt.size {
					t.Errorf("candidate %v holds %d devices, expect %d", ids, len(subset.Ids), tt.size)
				}
			}
		})
	}
}

// TestCandidateSubsetsCoverEveryCombination checks the dedup drops only
// duplicates, by comparing against every combination of whole GPUs that can
// satisfy the request.
func TestCandidateSubsetsCoverEveryCombination(t *testing.T) {
	ti := &testInfo{}
	devs := ti.getSyntheticDevices(5, 1)
	subsets, err := getCandidateDeviceSubsets(
		groupPartitionsByDevId(devs), devs, devs, nil, 3, map[int]map[int]int{})
	if err != nil {
		t.Fatalf("getCandidateDeviceSubsets failed: %s", err)
	}

	got := make(map[string]struct{}, len(subsets))
	for _, subset := range subsets {
		ids := make([]int, len(subset.Ids))
		copy(ids, subset.Ids)
		sort.Ints(ids)
		got[fmt.Sprint(ids)] = struct{}{}
	}

	// node ids are 2..6, so every 3 of them must be offered
	for i := 2; i <= 6; i++ {
		for j := i + 1; j <= 6; j++ {
			for k := j + 1; k <= 6; k++ {
				key := fmt.Sprint([]int{i, j, k})
				if _, exists := got[key]; !exists {
					t.Errorf("combination %s is missing from the candidates", key)
				}
			}
		}
	}
	if len(got) != 10 {
		t.Errorf("got %d distinct combinations, expect C(5,3) = 10", len(got))
	}
}

// TestCandidateSubsetsScaleOnLargePartitionedNode covers a node larger than the
// fixtures in testdata, where the permutational walk did not terminate.
func TestCandidateSubsetsScaleOnLargePartitionedNode(t *testing.T) {
	ti := &testInfo{}
	devs := ti.getSyntheticDevices(12, 8)

	done := make(chan int, 1)
	go func() {
		subsets, _ := getCandidateDeviceSubsets(
			groupPartitionsByDevId(devs), devs, devs, nil, 64, map[int]map[int]int{})
		done <- len(subsets)
	}()

	select {
	case count := <-done:
		// C(12,8) = 495
		if count != 495 {
			t.Errorf("got %d candidate subsets for 12 gpus of 8 partitions, expect C(12,8) = 495", count)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("enumerating candidates for 96 devices did not finish within 10s")
	}
}

// TestCandidateSubsetsWithTruncatedGroup covers a request that does not divide
// evenly into whole GPUs, where the gpu that gets truncated changes which
// devices the candidate holds and must therefore stay distinct.
func TestCandidateSubsetsWithTruncatedGroup(t *testing.T) {
	ti := &testInfo{}
	devs := ti.getSyntheticDevices(4, 4) // node ids 2..17, four gpus of four
	subsets, err := getCandidateDeviceSubsets(
		groupPartitionsByDevId(devs), devs, devs, nil, 10, map[int]map[int]int{})
	if err != nil {
		t.Fatalf("getCandidateDeviceSubsets failed: %s", err)
	}

	seen := make(map[string]struct{}, len(subsets))
	for _, subset := range subsets {
		ids := make([]int, len(subset.Ids))
		copy(ids, subset.Ids)
		sort.Ints(ids)
		key := fmt.Sprint(ids)
		if _, duplicate := seen[key]; duplicate {
			t.Errorf("device set %v was offered more than once", ids)
		}
		seen[key] = struct{}{}
		if len(subset.Ids) != 10 {
			t.Errorf("candidate %v holds %d devices, expect 10", ids, len(subset.Ids))
		}
	}

	// two gpus whole plus two of a third, so the truncated gpu must vary
	if len(seen) < 2 {
		t.Errorf("got %d distinct candidates, expect the truncated gpu to vary", len(seen))
	}
	t.Logf("%d distinct candidates for 10 of 16 devices across 4 gpus", len(seen))
}
