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

package allocator

import (
	"fmt"
	"math"
	"sort"

	"github.com/golang/glog"
)

/**
*  Best effort policy tries to come up with a subset of allocatable GPUs with best possible weight(connectivity).
*  We calculate weight of every GPU pair. The weight takes into account below information:
*  1. Type of link between the GPUs(XGMI or PCIE)
*  2. For partitioned GPUs, it tries to assign weights based on whether partitions are of same GPU or different GPUs
*  3. If both GPUs are part of same numa node or not
*  Pair with lower weight takes higher precedence. We calculate the sum of weights b/n individual pair within a given
*  subset and come up with total score for the subset. Subset with lowest score is given preference during allocation.
**/

const (
	invalidSize         = "allocation size can not be negative"
	invalidAvailable    = "available devices count less than allocation size"
	invalidRequired     = "must_include devices size is more than allocation size"
	invalidReqAvailable = "must_include length should be less than or equal to avilable device size"
	invalidInit         = "Init method must be called before Allocate"
	noCandidateFound    = "No candidate subset found with matching criteria"
)

type BestEffortPolicy struct {
	devices    map[string]*Device
	p2pWeights map[int]map[int]int
}

func NewBestEffortPolicy() *BestEffortPolicy {
	return &BestEffortPolicy{
		devices:    make(map[string]*Device),
		p2pWeights: make(map[int]map[int]int),
	}
}

func (b *BestEffortPolicy) getDevicesFromIds(ids []string) []*Device {
	var res []*Device
	for _, id := range ids {
		res = append(res, b.devices[id])
	}
	return res
}

// Init initializes pair wise weights of all devices and stores in-memory
func (b *BestEffortPolicy) Init(devs []*Device, topoDir string) error {
	err := fetchAllPairWeights(devs, b.p2pWeights, topoDir)
	if err == nil {
		for idx := range devs {
			b.devices[devs[idx].Id] = devs[idx]
		}
	}
	return err
}

func (b *BestEffortPolicy) Allocate(availableIds, requiredIds []string, size int) ([]string, error) {
	outset := []string{}
	if size <= 0 {
		return outset, fmt.Errorf(invalidSize)
	}

	if len(availableIds) < size {
		return outset, fmt.Errorf(invalidAvailable)
	}

	if len(requiredIds) > size {
		return outset, fmt.Errorf(invalidRequired)
	}

	if len(requiredIds) > len(availableIds) {
		return outset, fmt.Errorf(invalidReqAvailable)
	}

	if len(b.devices) == 0 {
		return outset, fmt.Errorf(invalidInit)
	}

	if len(availableIds) == size {
		return availableIds, nil
	}

	if len(b.p2pWeights) == 0 {
		return outset, fmt.Errorf(invalidInit)
	}

	available := b.getDevicesFromIds(availableIds)
	required := b.getDevicesFromIds(requiredIds)
	allSubsets, err := getAllDeviceSubsets(available, size, b.p2pWeights)

	if err != nil {
		return outset, err
	}

	var requiredNodeIds []int
	for i := 0; i < len(required); i++ {
		requiredNodeIds = append(requiredNodeIds, required[i].NodeId)
	}
	sort.Slice(requiredNodeIds, func(i, j int) bool {
		return requiredNodeIds[i] < requiredNodeIds[j]
	})

	bestScore := math.MaxInt32
	var candidate *DeviceSet
	for _, subset := range allSubsets {
		if !setContainsAll(subset.Ids, requiredNodeIds) {
			continue
		}
		if subset.TotalWeight < bestScore {
			candidate = subset
			bestScore = subset.TotalWeight
		}
	}
	for _, id := range candidate.Ids {
		for _, d := range available {
			if d.NodeId == id {
				outset = append(outset, d.Id)
				break
			}
		}
	}
	glog.Infof("best device subset:%v best score:%v", outset, candidate.TotalWeight)
	return outset, nil
}
