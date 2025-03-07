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

func TestBestPolicyAllocator(t *testing.T) {
	folderPath := "../../../testdata/topology-parsing-mi308/topology/nodes"
	devices := getTestDevices()
	a := NewBestEffortPolicy()
	a.Init(devices, folderPath)
	fmt.Printf("devices length = %d", len(devices))
	fmt.Printf("p2pWeights length = %d", len(a.p2pWeights))
	available := []string{"test2", "test3", "test4", "test5", "test6", "test7"}
	must_include := []string{"test3", "test4", "test5"}
	result, err := a.Allocate(available, must_include, 4)
	if err != nil {
		t.Errorf("expected Allocate method to pass. But failed with error %v", err)
	}
	if len(result) != 4 {
		t.Errorf("expected result to have 4 devices. But got %d", len(result))
	}
	fmt.Println(result)
}
