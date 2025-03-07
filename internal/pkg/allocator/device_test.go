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

func getTestDevices() []*Device {
	var res []*Device
	for i := 2; i < 34; i++ {
		res = append(res, &Device{
			Id:       fmt.Sprintf("test%d", i),
			NodeId:   i,
			NumaNode: i / 16,
			DevId:    i / 8,
		})
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
	folderPath := "../../../testdata/topology-parsing-mi308/topology/nodes"
	p2pWeights := make(map[int]map[int]int)
	devices := getTestDevices()
	err := fetchAllPairWeights(devices, p2pWeights, folderPath)
	if err != nil {
		t.Errorf("fetchAllPairWeights call failed. Error:%v", err)
	}
	if len(p2pWeights) != 31 {
		t.Errorf("expected p2pWeights length to be 31, but got %d\n", len(p2pWeights))
	}
}

func TestGetSubsetsMethod(t *testing.T) {
	folderPath := "../../../testdata/topology-parsing-mi308/topology/nodes"
	p2pWeights := make(map[int]map[int]int)
	devices := getTestDevices()
	err := fetchAllPairWeights(devices, p2pWeights, folderPath)
	if err != nil {
		t.Errorf("fetchAllPairWeights call failed. Error:%v", err)
	}

	subsets, err := getAllDeviceSubsets(devices, 3, p2pWeights)
	if err != nil {
		t.Errorf("expected getAllDeviceSubsets to pass. But got error %v", err)
	}
	if len(subsets) != 4960 {
		t.Errorf("expected subsets length to be 4960 but got %d", len(subsets))
	}
	subsets, err = getAllDeviceSubsets(devices, 2, p2pWeights)
	if err != nil {
		t.Errorf("expected getAllDeviceSubsets to pass. But got error %v", err)
	}
	if len(subsets) != 496 {
		t.Errorf("expected subsets length to be 496 but got %d", len(subsets))
	}
}
