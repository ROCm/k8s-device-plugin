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

// TestUnknownLinkWeightIsWorseThanAnyRecordedPair enumerates every score
// calculatePairWeight can produce and asserts the unknown-link fallback stays
// above all of them, so an undiscovered pair can never sort first.
func TestUnknownLinkWeightIsWorseThanAnyRecordedPair(t *testing.T) {
	// link types 11 and 2 are XGMI and PCIe, anything else takes otherLinkWeight
	linkTypes := []int{11, 2, 0, 3, 99}
	worst := 0

	for _, sameDev := range []bool{true, false} {
		for _, sameNuma := range []bool{true, false} {
			for _, linkType := range linkTypes {
				from := &Device{DevId: "a", NumaNode: 0}
				to := &Device{DevId: "a", NumaNode: 0}
				if !sameDev {
					to.DevId = "b"
				}
				if !sameNuma {
					to.NumaNode = 1
				}
				if w := calculatePairWeight(from, to, linkType); w > worst {
					worst = w
				}
			}
		}
	}

	if unknownLinkWeight <= worst {
		t.Errorf("unknownLinkWeight is %d but a recorded pair can score up to %d, so an unrecorded pair would be preferred",
			unknownLinkWeight, worst)
	}
}

func TestAddDeviceToSubsetScoresUnrecordedPairWorst(t *testing.T) {
	recordedWeight := sameDevIdWeight + xgmiLinkWeight + sameNumaNodeWeight
	p2pWeights := map[int]map[int]int{2: {3: recordedWeight}}
	base := NewDeviceSet([]int{2}, []int{0}, 0, 0)

	tests := []struct {
		name       string
		devId      int
		expectAdds int
	}{
		{name: "recorded pair", devId: 3, expectAdds: recordedWeight},
		{name: "pair absent from the row", devId: 99, expectAdds: unknownLinkWeight},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addDeviceToSubsetAndUpdateWeight(base, tt.devId, 0, p2pWeights)
			if got.TotalWeight != tt.expectAdds {
				t.Errorf("TotalWeight = %d, expect %d", got.TotalWeight, tt.expectAdds)
			}
		})
	}

	// The whole row missing must behave like a missing entry in an existing row.
	noRow := addDeviceToSubsetAndUpdateWeight(NewDeviceSet([]int{7}, []int{0}, 0, 0), 8, 0, p2pWeights)
	if noRow.TotalWeight != unknownLinkWeight {
		t.Errorf("TotalWeight = %d for a pair with no row at all, expect %d", noRow.TotalWeight, unknownLinkWeight)
	}
}

// TestCandidateSubsetsPreferRecordedLinks covers a topology split into two XGMI
// islands with nothing recorded between them, which is where an unrecorded pair
// scoring zero used to win.
func TestCandidateSubsetsPreferRecordedLinks(t *testing.T) {
	devs := []*Device{
		{Id: "gpu0", NodeId: 2, DevId: "0", NumaNode: 0},
		{Id: "gpu1", NodeId: 3, DevId: "1", NumaNode: 0},
		{Id: "gpu2", NodeId: 4, DevId: "2", NumaNode: 1},
		{Id: "gpu3", NodeId: 5, DevId: "3", NumaNode: 1},
	}
	linked := differentDevIdWeight + xgmiLinkWeight + sameNumaNodeWeight
	p2pWeights := map[int]map[int]int{
		2: {3: linked}, // island A
		4: {5: linked}, // island B
	}

	subsets, err := getCandidateDeviceSubsets(groupPartitionsByDevId(devs), devs, devs, nil, 2, p2pWeights)
	if err != nil {
		t.Fatalf("getCandidateDeviceSubsets failed: %s", err)
	}
	if len(subsets) == 0 {
		t.Fatal("getCandidateDeviceSubsets returned no candidate")
	}

	best := subsets[0]
	for _, subset := range subsets {
		if subset.TotalWeight < best.TotalWeight {
			best = subset
		}
	}

	sort.Ints(best.Ids)
	withinIsland := (best.Ids[0] == 2 && best.Ids[1] == 3) || (best.Ids[0] == 4 && best.Ids[1] == 5)
	if !withinIsland {
		t.Errorf("best subset is %v with weight %d, expect an XGMI connected pair from one island",
			best.Ids, best.TotalWeight)
	}
}
