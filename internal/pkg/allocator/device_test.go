/**
# Copyright (c) Advanced Micro Devices, Inc. All rights reserved.
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
				DevId:    i,
			})
			nodeId = nodeId + 1
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
			expectedSubsetsLength: 6,
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
