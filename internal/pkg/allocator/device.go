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
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"github.com/golang/glog"
)

const (
	topoRootPath = "/sys/class/kfd/kfd/topology/nodes"
)

// below scores/weights are used to determine the closeness/efficiency of communication between GPU pairs
const (
	// weight if GPUs/partitions belong to same GPU
	sameDevIdWeight = 10
	// weight if a pair is connected via XGMI link
	xgmiLinkWeight = 10
	// weight if GPU pair belongs to same numa node
	sameNumaNodeWeight = 10
	// weight if GPUs/partitions belong to different GPU.
	// In case of full GPUs, the weight is 3
	differentDevIdWeight = 20
	// weight if GPU pair belongs to different numa node
	differentNumaNodeWeight = 20
	// weight if a pair is connected via PCIE link
	pcieLinkWeight = 40
	// weight if a pair is connected via any other link apart from XGMI or PCIE
	otherLinkWeight = 50
)

type Device struct {
	Id               string
	NodeId           int
	NumaNode         int
	DevId            int
	Card             int
	RenderD          int
	ComputePartition string
	MemoryPartition  string
}

type DeviceSet struct {
	Ids         []int
	TotalWeight int
	LastIdx     int
}

func setContainsAll(set []int, subset []int) bool {
	if len(subset) > len(set) {
		return false
	}
	for _, dev := range subset {
		devFound := false
		for i := range set {
			if set[i] == dev {
				devFound = true
				break
			}
		}
		if !devFound {
			return false
		}
	}
	return true
}

func fetchTopoProperties(path string, re []*regexp.Regexp) ([]int, error) {
	f, e := os.Open(path)
	if e != nil {
		glog.Errorf("Unable to open properties file. Error:%v", e)
		return []int{0}, e
	}
	defer f.Close()

	res := make([]int, len(re))
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		for idx := range re {
			m := re[idx].FindStringSubmatch(scanner.Text())
			if m == nil {
				continue
			}
			v, err := strconv.ParseInt(m[1], 0, 32)
			if err != nil {
				glog.Errorf("Unable to parse properties file. Error:%v", err)
				return nil, err
			}
			res[idx] = int(v)
		}
	}

	return res, nil
}

func calculatePairWeight(from, to *Device, linkType int) int {
	weight := 0
	if from.DevId == to.DevId {
		weight = weight + sameDevIdWeight
	} else {
		weight = weight + differentDevIdWeight
	}

	if linkType == 11 { // link type 11 is xgmi
		weight = weight + xgmiLinkWeight
	} else if linkType == 2 { //link type 2 is PCIE
		weight = weight + pcieLinkWeight
	} else { // other link types are given higher weight
		weight = weight + otherLinkWeight
	}

	if from.NumaNode == to.NumaNode {
		weight = weight + sameNumaNodeWeight
	} else {
		weight = weight + differentNumaNodeWeight
	}
	return weight
}

func scanAndPopulatePeerWeights(fromPath string, devices []*Device, lookupNodes map[int]struct{}, p2pWeights map[int]map[int]int) error {
	paths, err1 := filepath.Glob(filepath.Join(fromPath, "io_links", "[0-9]*"))
	p2pPaths, err2 := filepath.Glob(filepath.Join(fromPath, "p2p_links", "[0-9]*"))
	if err1 != nil && err2 != nil {
		glog.Errorf("unable to fetch io_links and p2p_links folders. Error1:%v Error2:%v", err1, err2)
		return fmt.Errorf("Unable to Glob io_links and p2p_links paths")
	}
	if len(p2pPaths) > 0 {
		paths = append(paths, p2pPaths...)
	}
	re := []*regexp.Regexp{
		regexp.MustCompile(`node_from\s(\d+)`),
		regexp.MustCompile(`node_to\s(\d+)`),
		regexp.MustCompile(`type\s(\d+)`),
	}
	for _, topath := range paths {
		propFile := filepath.Join(topath, "properties")
		vals, err := fetchTopoProperties(propFile, re)
		if err != nil {
			continue
		}
		// to avoid duplicates in the map we make sure from < to
		var from, to int
		if vals[0] < vals[1] {
			from = vals[0]
			to = vals[1]
		} else {
			from = vals[1]
			to = vals[0]
		}
		if _, ok := lookupNodes[from]; !ok {
			continue
		}
		if _, ok := lookupNodes[to]; !ok {
			continue
		}

		var fromDev, toDev *Device
		devsFound := false
		for idx := range devices {
			if devices[idx].NodeId == from {
				fromDev = devices[idx]
			}
			if devices[idx].NodeId == to {
				toDev = devices[idx]
			}
			if fromDev != nil && toDev != nil {
				devsFound = true
				break
			}
		}
		if devsFound {
			if _, ok := p2pWeights[from]; !ok {
				p2pWeights[from] = make(map[int]int)
			}
			p2pWeights[from][to] = calculatePairWeight(fromDev, toDev, int(vals[2]))
		}
	}
	return nil
}

func fetchAllPairWeights(devices []*Device, p2pWeights map[int]map[int]int, folderPath string) error {
	if len(devices) == 0 {
		errMsg := fmt.Sprintf("Devices list is empty. Unable to calculate pair wise weights")
		glog.Info(errMsg)
		return fmt.Errorf(errMsg)
	}
	if folderPath == "" {
		folderPath = topoRootPath
	}
	paths, err := filepath.Glob(filepath.Join(folderPath, "[0-9]*"))
	if err != nil {
		return fmt.Errorf("unable to find gpu nodes under topo directory")
	}
	nodeIds := make(map[int]struct{})
	for idx := range devices {
		nodeIds[devices[idx].NodeId] = struct{}{}
	}
	drmRenderMinor := []*regexp.Regexp{regexp.MustCompile(`drm_render_minor\s(\d+)`)}
	for _, path := range paths {
		propFilePath := filepath.Join(path, "properties")
		vals, err := fetchTopoProperties(propFilePath, drmRenderMinor)
		// if drm_render_minor value is <= 0, then it's not a valid GPU/partition
		if err != nil || vals[0] <= 0 {
			continue
		}
		err = scanAndPopulatePeerWeights(path, devices, nodeIds, p2pWeights)
		if err != nil {

			return err
		}
	}
	return nil
}

func addDeviceToSubsetAndUpdateWeight(subset *DeviceSet, devId, devIdx int, p2pWeights map[int]map[int]int) *DeviceSet {
	currentWeight := subset.TotalWeight
	var from, to int
	ids := make([]int, 0)
	for _, d := range subset.Ids {
		if d < devId {
			from = d
			to = devId
		} else {
			from = devId
			to = d
		}
		currentWeight = currentWeight + p2pWeights[from][to]
	}
	ids = append(ids, subset.Ids...)
	ids = append(ids, devId)

	newSubset := NewDeviceSet(ids, currentWeight, devIdx)
	return newSubset
}

func NewDeviceSet(nodeIds []int, weight, lastIdx int) *DeviceSet {
	return &DeviceSet{
		Ids:         nodeIds,
		TotalWeight: weight,
		LastIdx:     lastIdx,
	}
}

func deriveSubsetsFromPrevLevel(subsets []*DeviceSet, availableIds []int, p2pWeights map[int]map[int]int) []*DeviceSet {
	outset := make([]*DeviceSet, 0)
	for _, subset := range subsets {
		start := subset.LastIdx
		for i := start + 1; i < len(availableIds); i++ {
			newsub := addDeviceToSubsetAndUpdateWeight(subset, availableIds[i], i, p2pWeights)
			outset = append(outset, newsub)
		}
	}
	return outset
}

func getAllDeviceSubsets(available []*Device, size int, p2pWeights map[int]map[int]int) ([]*DeviceSet, error) {
	if size <= 0 {
		return []*DeviceSet{}, fmt.Errorf("subset size should be positive integer")
	}

	if len(available) < size {
		return []*DeviceSet{}, fmt.Errorf("subset size is more than available devices")
	}

	var availableIds []int
	for i := 0; i < len(available); i++ {
		availableIds = append(availableIds, available[i].NodeId)
	}
	sort.Slice(availableIds, func(i, j int) bool {
		return availableIds[i] < availableIds[j]
	})

	subsets := make([][]*DeviceSet, size)
	//for level 0 create subsets with single element
	subsets[0] = make([]*DeviceSet, 0)
	for index, id := range availableIds {
		ids := []int{id}
		subsets[0] = append(subsets[0], NewDeviceSet(ids, 0, index))
	}

	for i := 1; i < size; i++ {
		currLevel := deriveSubsetsFromPrevLevel(subsets[i-1], availableIds, p2pWeights)
		subsets[i] = make([]*DeviceSet, 0)
		subsets[i] = append(subsets[i], currLevel...)
	}
	return subsets[size-1], nil
}
