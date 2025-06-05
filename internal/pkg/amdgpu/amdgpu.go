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

// Package amdgpu is a collection of utility functions to access various properties
// of AMD GPU via Linux kernel interfaces like sysfs and ioctl (using libdrm.)
package amdgpu

// #cgo pkg-config: libdrm libdrm_amdgpu
// #include <stdint.h>
// #include <xf86drm.h>
// #include <drm.h>
// #include <amdgpu.h>
// #include <amdgpu_drm.h>
import "C"
import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ROCm/k8s-device-plugin/internal/pkg/allocator"
	"github.com/ROCm/k8s-device-plugin/internal/pkg/exporter"
	"github.com/ROCm/k8s-device-plugin/internal/pkg/types"
	"github.com/golang/glog"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

// AMDGPUKFDImpl implements the DeviceImpl interface for container-based workloads using ROCm/KFD
type AMDGPUKFDImpl struct {
	initErr                error
	deviceMap              map[string]map[string]interface{}
	homogeneous            bool
	devList                map[string][]*pluginapi.Device
	resourceNamingStrategy string
}

func NewGPUKFDImpl(initParams map[string]interface{}) (types.DeviceImpl, error) {
	resourceNamingStrategy, _ := initParams[types.CmdLineResNamingStrategy]
	impl := &AMDGPUKFDImpl{
		resourceNamingStrategy: resourceNamingStrategy.(string),
	}

	if err := impl.Init(); err != nil {
		return nil, err
	}
	return impl, nil
}

func (i *AMDGPUKFDImpl) Init() error {
	var path = "/sys/class/kfd"
	if _, err := os.Stat(path); err != nil {
		i.initErr = fmt.Errorf("No amd gpu driver loaded")
		return i.initErr
	}
	i.deviceMap = GetAMDGPUs()
	i.homogeneous = IsHomogeneous(i.deviceMap)

	if !i.homogeneous && i.resourceNamingStrategy == types.ResourceNamingStrategySingle {
		i.initErr = fmt.Errorf("Partitions of different styles across GPUs in a node is not supported with single strategy. Please start device plugin with mixed strategy")
		return i.initErr
	}

	i.devList = make(map[string][]*pluginapi.Device)
	resources := i.GetResourceNames()
	for _, r := range resources {
		i.devList[r] = i.convertToPluginDeviceList(r)
	}
	return nil
}

func (i *AMDGPUKFDImpl) Start(ctx types.DevicePluginContext) error {
	var deviceList []*allocator.Device

	if i.initErr != nil {
		return i.initErr
	}

	for id, deviceData := range i.deviceMap {
		device := &allocator.Device{
			Id:                   id,
			Card:                 deviceData["card"].(int),
			RenderD:              deviceData["renderD"].(int),
			DevId:                deviceData["devID"].(string),
			ComputePartitionType: deviceData["computePartitionType"].(string),
			MemoryPartitionType:  deviceData["memoryPartitionType"].(string),
			NodeId:               deviceData["nodeId"].(int),
			NumaNode:             deviceData["numaNode"].(int),
		}
		deviceList = append(deviceList, device)
	}

	devAllocator := ctx.GetAllocator()
	err := devAllocator.Init(deviceList, "")
	if err != nil {
		glog.Errorf("allocator init failed for plugin %s. Falling back to kubelet default allocation. Error %v", ctx.ResourceName(), err)
		ctx.SetAllocatorError(true)
	}

	return nil
}

// GetResourceNames returns a slice of resource names
func (i *AMDGPUKFDImpl) GetResourceNames() (resources []string) {

	if i.initErr != nil {
		return
	}

	if len(i.deviceMap) == 0 {
		return
	}

	partitionCountMap := UniquePartitionConfigCount(i.deviceMap)

	// Check if the node is homogeneous
	if i.homogeneous {
		// Homogeneous node will report only "gpu" resource if strategy is single. If strategy is mixed, it will report resources under the partition type name
		if i.resourceNamingStrategy == types.ResourceNamingStrategySingle {
			resources = []string{types.DeviceTypeGPU}
		} else if i.resourceNamingStrategy == types.ResourceNamingStrategyMixed {
			if len(partitionCountMap) == 0 {
				// If partitioning is not supported on the node, we should report resources under "gpu" regardless of the strategy
				resources = []string{types.DeviceTypeGPU}
			} else {
				for partitionType, count := range partitionCountMap {
					if count > 0 {
						resources = append(resources, partitionType)
					}
				}
			}
		}
	} else {
		// Heterogeneous node reports resources based on partition types if strategy is mixed. Heterogeneous is not allowed if Strategy is single
		// Strategy is mixed in this case
		for partitionType, count := range partitionCountMap {
			if count > 0 {
				resources = append(resources, partitionType)
			}
		}
	}

	return resources
}

// GetOptions returns the device plugin options supported for the resource
func (i *AMDGPUKFDImpl) GetOptions(ctx types.DevicePluginContext) (*pluginapi.DevicePluginOptions, error) {

	if i.initErr != nil {
		return nil, i.initErr
	}

	if ctx.GetAllocatorError() {
		return &pluginapi.DevicePluginOptions{}, nil
	}
	return &pluginapi.DevicePluginOptions{
		GetPreferredAllocationAvailable: true,
	}, nil
}

// Enumerate discovers available devices using the KFD interface
func (i *AMDGPUKFDImpl) Enumerate(ctx types.DevicePluginContext) ([]*pluginapi.Device, error) {

	if i.initErr != nil {
		return nil, i.initErr
	}

	glog.Infof("Found %d AMDGPUs", len(i.deviceMap))

	return i.devList[ctx.ResourceName()], nil
}

func (i *AMDGPUKFDImpl) convertToPluginDeviceList(resource string) []*pluginapi.Device {
	devs := make([]*pluginapi.Device, len(i.deviceMap))
	// Initialize a map to store partitionType based device list
	resourceTypeDevs := make(map[string][]*pluginapi.Device)

	if i.homogeneous {
		idx := 0
		for id, device := range i.deviceMap {
			dev := &pluginapi.Device{
				ID:     id,
				Health: pluginapi.Healthy,
			}
			devs[idx] = dev
			idx++

			numas := []int64{int64(device["numaNode"].(int))}
			glog.Infof("Watching GPU with bus ID: %s NUMA Node: %+v", id, numas)

			numaNodes := make([]*pluginapi.NUMANode, len(numas))
			for j, v := range numas {
				numaNodes[j] = &pluginapi.NUMANode{
					ID: int64(v),
				}
			}

			dev.Topology = &pluginapi.TopologyInfo{
				Nodes: numaNodes,
			}
		}
	} else {
		// Iterate through deviceCountMap and create empty lists for each partitionType whose count is > 0 with variable name same as partitionType
		for id, device := range i.deviceMap {
			dev := &pluginapi.Device{
				ID:     id,
				Health: pluginapi.Healthy,
			}
			// Append a device belonging to a certain partition type to its respective list
			partitionType := device["computePartitionType"].(string) + "_" + device["memoryPartitionType"].(string)
			resourceTypeDevs[partitionType] = append(resourceTypeDevs[partitionType], dev)

			numas := []int64{int64(device["numaNode"].(int))}
			glog.Infof("Watching GPU with bus ID: %s NUMA Node: %+v", id, numas)

			numaNodes := make([]*pluginapi.NUMANode, len(numas))
			for j, v := range numas {
				numaNodes[j] = &pluginapi.NUMANode{
					ID: int64(v),
				}
			}

			dev.Topology = &pluginapi.TopologyInfo{
				Nodes: numaNodes,
			}
		}
		// Send the appropriate list of devices based on the partitionType
		if devList, exists := resourceTypeDevs[resource]; exists {
			devs = devList
		}
	}

	return devs
}

// Allocate returns allocation details for container-based workloads using KFD
func (i *AMDGPUKFDImpl) Allocate(ctx types.DevicePluginContext, r *pluginapi.AllocateRequest) (resp *pluginapi.AllocateResponse, err error) {

	if i.initErr != nil {
		return nil, i.initErr
	}

	var response pluginapi.AllocateResponse
	var car pluginapi.ContainerAllocateResponse
	var dev *pluginapi.DeviceSpec

	for _, req := range r.ContainerRequests {
		car = pluginapi.ContainerAllocateResponse{}

		// Currently, there are only 1 /dev/kfd per nodes regardless of the # of GPU available
		// for compute/rocm/HSA use cases
		dev = new(pluginapi.DeviceSpec)
		dev.HostPath = "/dev/kfd"
		dev.ContainerPath = "/dev/kfd"
		dev.Permissions = "rw"
		car.Devices = append(car.Devices, dev)

		for _, id := range req.DevicesIDs {
			glog.Infof("Allocating device ID: %s", id)

			for k, v := range i.deviceMap[id] {
				// Map struct previously only had 'card' and 'renderD' and only those are paths to be appended as before
				if k != "card" && k != "renderD" {
					continue
				}
				devpath := fmt.Sprintf("/dev/dri/%s%d", k, v)
				dev = new(pluginapi.DeviceSpec)
				dev.HostPath = devpath
				dev.ContainerPath = devpath
				dev.Permissions = "rw"
				car.Devices = append(car.Devices, dev)
			}
		}

		response.ContainerResponses = append(response.ContainerResponses, &car)
	}

	return &response, nil
}

// GetPreferredAllocation returns the preferred allocation response for a given resource and request
func (i *AMDGPUKFDImpl) GetPreferredAllocation(ctx types.DevicePluginContext, req *pluginapi.PreferredAllocationRequest) (resp *pluginapi.PreferredAllocationResponse, err error) {

	if i.initErr != nil {
		return nil, i.initErr
	}

	response := &pluginapi.PreferredAllocationResponse{}
	for _, req := range req.ContainerRequests {
		allocated_ids, err := ctx.GetAllocator().Allocate(req.AvailableDeviceIDs, req.MustIncludeDeviceIDs, int(req.AllocationSize))
		if err != nil {
			glog.Errorf("unable to get preferred allocation list. Error:%v", err)
			return nil, fmt.Errorf("unable to get preferred allocation list. Error:%v", err)
		}
		resp := &pluginapi.ContainerPreferredAllocationResponse{
			DeviceIDs: allocated_ids,
		}
		response.ContainerResponses = append(response.ContainerResponses, resp)
	}
	return response, nil
}

// UpdateHealth returns a health status for the devices of a given resource
func (i *AMDGPUKFDImpl) UpdateHealth(ctx types.DevicePluginContext) (devices []*pluginapi.Device, err error) {

	if i.initErr != nil {
		return nil, i.initErr
	}

	var health = pluginapi.Unhealthy

	if simpleHealthCheck() {
		health = pluginapi.Healthy
	}

	devs, ok := i.devList[ctx.ResourceName()]
	// update with per device GPU health status
	if i.homogeneous {
		exporter.PopulatePerGPUDHealth(devs, health)
	} else {
		if ok {
			exporter.PopulatePerGPUDHealth(devs, health)
		}
	}

	return devs, nil
}

// FamilyID to String convert AMDGPU_FAMILY_* into string
// AMDGPU_FAMILY_* as defined in https://github.com/torvalds/linux/blob/master/include/uapi/drm/amdgpu_drm.h#L986
func FamilyIDtoString(familyId uint32) (string, error) {
	switch familyId {
	case C.AMDGPU_FAMILY_SI:
		return "SI", nil
	case C.AMDGPU_FAMILY_CI:
		return "CI", nil
	case C.AMDGPU_FAMILY_KV:
		return "KV", nil
	case C.AMDGPU_FAMILY_VI:
		return "VI", nil
	case C.AMDGPU_FAMILY_CZ:
		return "CZ", nil
	case C.AMDGPU_FAMILY_AI:
		return "AI", nil
	case C.AMDGPU_FAMILY_RV:
		return "RV", nil
	case C.AMDGPU_FAMILY_NV:
		return "NV", nil
	case C.AMDGPU_FAMILY_VGH:
		return "VGH", nil
	case C.AMDGPU_FAMILY_GC_11_0_0:
		return "GC_11_0_0", nil
	case C.AMDGPU_FAMILY_YC:
		return "YC", nil
	case C.AMDGPU_FAMILY_GC_11_0_1:
		return "GC_11_0_1", nil
	case C.AMDGPU_FAMILY_GC_10_3_6:
		return "GC_10_3_6", nil
	case C.AMDGPU_FAMILY_GC_10_3_7:
		return "GC_10_3_7", nil
	case C.AMDGPU_FAMILY_GC_11_5_0:
		return "GC_11_5_0", nil
	default:
		ret := ""
		err := fmt.Errorf("Unknown Family ID: %d", familyId)
		return ret, err
	}

}

func GetCardFamilyName(cardName string) (string, error) {
	devHandle, err := openAMDGPU(cardName)
	if err != nil {
		return "", err
	}
	defer C.amdgpu_device_deinitialize(devHandle)

	var info C.struct_amdgpu_gpu_info
	rc := C.amdgpu_query_gpu_info(devHandle, &info)

	if rc < 0 {
		return "", fmt.Errorf("Fail to get FamilyID %s: %d", cardName, rc)
	}

	return FamilyIDtoString(uint32(info.family_id))
}

func GetDevIdsFromTopology(topoRootParam ...string) map[int]string {
	topoRoot := "/sys/class/kfd/kfd"
	if len(topoRootParam) == 1 {
		topoRoot = topoRootParam[0]
	}

	renderDevIds := make(map[int]string)
	var nodeFiles []string
	var err error

	if nodeFiles, err = filepath.Glob(topoRoot + "/topology/nodes/*/properties"); err != nil {
		glog.Fatalf("glob error: %s", err)
		return renderDevIds
	}

	for _, nodeFile := range nodeFiles {
		glog.Info("Parsing " + nodeFile)
		v, e := ParseTopologyProperties(nodeFile, topoDrmRenderMinorRe)
		if e != nil {
			glog.Error(e)
			continue
		}

		if v <= 0 {
			continue
		}

		// Fetch unique_id value from properties file.
		// This unique_id is the same for the real gpu as well as its partitions so it will be used to associate the partitions to the real gpu
		devID, e := ParseTopologyPropertiesString(nodeFile, topoUniqueIdRe)
		if e != nil {
			glog.Error(e)
			continue
		}

		renderDevIds[int(v)] = devID
	}

	return renderDevIds
}

// GetAMDGPUs return a map of AMD GPU on a node identified by the part of the pci address
func GetAMDGPUs() map[string]map[string]interface{} {
	if _, err := os.Stat("/sys/module/amdgpu/drivers/"); err != nil {
		glog.Warningf("amdgpu driver unavailable: %s", err)
		return make(map[string]map[string]interface{})
	}

	//ex: /sys/module/amdgpu/drivers/pci:amdgpu/0000:19:00.0
	matches, _ := filepath.Glob("/sys/module/amdgpu/drivers/pci:amdgpu/[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]:*")

	devID := ""
	devices := make(map[string]map[string]interface{})
	card, renderD, nodeId := 0, 128, 0
	renderDevIds := GetDevIdsFromTopology()
	renderNodeIds := GetNodeIdsFromTopology()

	for _, path := range matches {
		computePartitionFile := filepath.Join(path, "current_compute_partition")
		memoryPartitionFile := filepath.Join(path, "current_memory_partition")
		numaNodeFile := filepath.Join(path, "numa_node")

		computePartitionType, memoryPartitionType := "", ""
		numaNode := -1

		// Read the compute partition
		if data, err := ioutil.ReadFile(computePartitionFile); err == nil {
			computePartitionType = strings.ToLower(strings.TrimSpace(string(data)))
		} else {
			glog.Warningf("Failed to read 'current_compute_partition' file at %s: %s", computePartitionFile, err)
		}

		// Read the memory partition
		if data, err := ioutil.ReadFile(memoryPartitionFile); err == nil {
			memoryPartitionType = strings.ToLower(strings.TrimSpace(string(data)))
		} else {
			glog.Warningf("Failed to read 'current_memory_partition' file at %s: %s", memoryPartitionFile, err)
		}

		if data, err := ioutil.ReadFile(numaNodeFile); err == nil {
			numaNodeStr := strings.TrimSpace(string(data))
			numaNode, err = strconv.Atoi(numaNodeStr)
			if err != nil {
				glog.Warningf("Failed to convert 'numa_node' value to int: %s", err)
				continue
			}
		} else {
			glog.Warningf("Failed to read 'numa_node' file at %s: %s", numaNodeFile, err)
			continue
		}

		glog.Info(path)
		devPaths, _ := filepath.Glob(path + "/drm/*")

		for _, devPath := range devPaths {
			switch name := filepath.Base(devPath); {
			case name[0:4] == "card":
				card, _ = strconv.Atoi(name[4:])
			case name[0:7] == "renderD":
				renderD, _ = strconv.Atoi(name[7:])
				if val, exists := renderDevIds[renderD]; exists {
					devID = val
				}
				if id, exists := renderNodeIds[renderD]; exists {
					nodeId = id
				}
			}

		}
		// add devID so that we can identify later which gpu should get reported under which resource type
		devices[filepath.Base(path)] = map[string]interface{}{"card": card, "renderD": renderD, "devID": devID, "computePartitionType": computePartitionType, "memoryPartitionType": memoryPartitionType, "numaNode": numaNode, "nodeId": nodeId}
	}

	// certain products have additional devices (such as MI300's partitions)
	//ex: /sys/devices/platform/amdgpu_xcp_30
	platformMatches, _ := filepath.Glob("/sys/devices/platform/amdgpu_xcp_*")

	for _, path := range platformMatches {
		glog.Info(path)
		devPaths, _ := filepath.Glob(path + "/drm/*")

		computePartitionType, memoryPartitionType := "", ""
		numaNode := -1

		for _, devPath := range devPaths {
			switch name := filepath.Base(devPath); {
			case name[0:4] == "card":
				card, _ = strconv.Atoi(name[4:])
			case name[0:7] == "renderD":
				renderD, _ = strconv.Atoi(name[7:])
				if val, exists := renderDevIds[renderD]; exists {
					devID = val
				}
				// Set the computePartitionType and memoryPartitionType from the real GPU or from other partitions using the common devID
				for _, device := range devices {
					if device["devID"] == devID {
						if device["computePartitionType"].(string) != "" && device["memoryPartitionType"].(string) != "" {
							computePartitionType = device["computePartitionType"].(string)
							memoryPartitionType = device["memoryPartitionType"].(string)
							numaNode = device["numaNode"].(int)
							break
						}
					}
				}
				if id, exists := renderNodeIds[renderD]; exists {
					nodeId = id
				}
			}
		}
		// This is needed because some of the visible renderD are actually not valid
		// Their validity depends on topology information from KFD

		if _, exists := renderDevIds[renderD]; !exists {
			continue
		}
		if numaNode == -1 {
			continue
		}
		devices[filepath.Base(path)] = map[string]interface{}{"card": card, "renderD": renderD, "devID": devID, "computePartitionType": computePartitionType, "memoryPartitionType": memoryPartitionType, "numaNode": numaNode, "nodeId": nodeId}
	}
	glog.Infof("Devices map: %v", devices)
	return devices
}

func UniquePartitionConfigCount(devices map[string]map[string]interface{}) map[string]int {
	partitionCountMap := make(map[string]int)

	for _, device := range devices {
		computePartitionType := device["computePartitionType"].(string)
		memoryPartitionType := device["memoryPartitionType"].(string)

		if computePartitionType != "" && memoryPartitionType != "" {
			overallPartition := computePartitionType + "_" + memoryPartitionType
			partitionCountMap[overallPartition]++
		}
	}

	glog.Infof("Partition counts: %v", partitionCountMap)
	return partitionCountMap
}

// IsHomogeneous checks if the device map is homogeneous based on the partition types
func IsHomogeneous(deviceMap map[string]map[string]interface{}) bool {
	partitionCountMap := UniquePartitionConfigCount(deviceMap)
	// Homogeneous if the map is empty or contains exactly one partition type
	return len(partitionCountMap) <= 1
}

func IsComputePartitionSupported() bool {
	// Finding GPU paths using the same way its done in other functions like GetAMDGPUs()
	matches, _ := filepath.Glob("/sys/module/amdgpu/drivers/pci:amdgpu/[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]:*")
	if len(matches) == 0 {
		return false
	}
	// Check any one GPU to see if it supports partition (All GPU's are of same model on the node)
	path := matches[0]
	computePartitionFile := filepath.Join(path, "available_compute_partition")

	if _, err := os.Stat(computePartitionFile); err != nil {
		return false
	}

	// If file exists, then compute partition is supported
	return true
}

func IsMemoryPartitionSupported() bool {
	// Finding GPU paths using the same way its done in other functions like GetAMDGPUs()
	matches, _ := filepath.Glob("/sys/module/amdgpu/drivers/pci:amdgpu/[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]:*")
	if len(matches) == 0 {
		return false
	}
	// Check any one GPU to see if it supports partition (All GPU's are of same model on the node)
	path := matches[0]
	memoryPartitionFile := filepath.Join(path, "available_memory_partition")

	if _, err := os.Stat(memoryPartitionFile); err != nil {
		return false
	}
	// If file exists, then memory partition is supported
	return true
}

// AMDGPU check if a particular card is an AMD GPU by checking the device's vendor ID
func AMDGPU(cardName string) bool {
	sysfsVendorPath := "/sys/class/drm/" + cardName + "/device/vendor"
	b, err := ioutil.ReadFile(sysfsVendorPath)
	if err == nil {
		vid := strings.TrimSpace(string(b))

		// AMD vendor ID is 0x1002
		if "0x1002" == vid {
			return true
		}
	} else {
		glog.Errorf("Error opening %s: %s", sysfsVendorPath, err)
	}
	return false
}

func openAMDGPU(cardName string) (C.amdgpu_device_handle, error) {
	if !AMDGPU(cardName) {
		return nil, fmt.Errorf("%s is not an AMD GPU", cardName)
	}
	devPath := "/dev/dri/" + cardName

	dev, err := os.Open(devPath)

	if err != nil {
		return nil, fmt.Errorf("Fail to open %s: %s", devPath, err)
	}
	defer dev.Close()

	devFd := C.int(dev.Fd())

	var devHandle C.amdgpu_device_handle
	var major C.uint32_t
	var minor C.uint32_t

	rc := C.amdgpu_device_initialize(devFd, &major, &minor, &devHandle)

	if rc < 0 {
		return nil, fmt.Errorf("Fail to initialize %s: %d", devPath, err)
	}
	glog.Infof("Initialized AMD GPU version: major %d, minor %d", major, minor)

	return devHandle, nil

}

// DevFunctional does a simple check on whether a particular GPU is working
// by attempting to open the device
func DevFunctional(cardName string) bool {
	devHandle, err := openAMDGPU(cardName)
	if err != nil {
		glog.Errorf("%s", err)
		return false
	}
	defer C.amdgpu_device_deinitialize(devHandle)

	return true
}

// GetFirmwareVersions obtain a subset of firmware and feature version via libdrm
// amdgpu_query_firmware_version
func GetFirmwareVersions(cardName string) (map[string]uint32, map[string]uint32, error) {
	devHandle, err := openAMDGPU(cardName)
	if err != nil {
		return map[string]uint32{}, map[string]uint32{}, err
	}
	defer C.amdgpu_device_deinitialize(devHandle)

	var ver C.uint32_t
	var feat C.uint32_t

	featVersions := map[string]uint32{}
	fwVersions := map[string]uint32{}

	C.amdgpu_query_firmware_version(devHandle, C.AMDGPU_INFO_FW_VCE, 0, 0, &ver, &feat)
	featVersions["VCE"] = uint32(feat)
	fwVersions["VCE"] = uint32(ver)
	C.amdgpu_query_firmware_version(devHandle, C.AMDGPU_INFO_FW_UVD, 0, 0, &ver, &feat)
	featVersions["UVD"] = uint32(feat)
	fwVersions["UVD"] = uint32(ver)
	C.amdgpu_query_firmware_version(devHandle, C.AMDGPU_INFO_FW_GMC, 0, 0, &ver, &feat)
	featVersions["MC"] = uint32(feat)
	fwVersions["MC"] = uint32(ver)
	C.amdgpu_query_firmware_version(devHandle, C.AMDGPU_INFO_FW_GFX_ME, 0, 0, &ver, &feat)
	featVersions["ME"] = uint32(feat)
	fwVersions["ME"] = uint32(ver)
	C.amdgpu_query_firmware_version(devHandle, C.AMDGPU_INFO_FW_GFX_PFP, 0, 0, &ver, &feat)
	featVersions["PFP"] = uint32(feat)
	fwVersions["PFP"] = uint32(ver)
	C.amdgpu_query_firmware_version(devHandle, C.AMDGPU_INFO_FW_GFX_CE, 0, 0, &ver, &feat)
	featVersions["CE"] = uint32(feat)
	fwVersions["CE"] = uint32(ver)
	C.amdgpu_query_firmware_version(devHandle, C.AMDGPU_INFO_FW_GFX_RLC, 0, 0, &ver, &feat)
	featVersions["RLC"] = uint32(feat)
	fwVersions["RLC"] = uint32(ver)
	C.amdgpu_query_firmware_version(devHandle, C.AMDGPU_INFO_FW_GFX_MEC, 0, 0, &ver, &feat)
	featVersions["MEC"] = uint32(feat)
	fwVersions["MEC"] = uint32(ver)
	C.amdgpu_query_firmware_version(devHandle, C.AMDGPU_INFO_FW_SMC, 0, 0, &ver, &feat)
	featVersions["SMC"] = uint32(feat)
	fwVersions["SMC"] = uint32(ver)
	C.amdgpu_query_firmware_version(devHandle, C.AMDGPU_INFO_FW_SDMA, 0, 0, &ver, &feat)
	featVersions["SDMA0"] = uint32(feat)
	fwVersions["SDMA0"] = uint32(ver)

	return featVersions, fwVersions, nil
}

// ParseTopologyProperties parse for a property value in kfd topology file
// The format is usually one entry per line <name> <value>.  Examples in
// testdata/topology-parsing/.
func ParseTopologyProperties(path string, re *regexp.Regexp) (int64, error) {
	f, e := os.Open(path)
	if e != nil {
		return 0, e
	}

	e = errors.New("Topology property not found.  Regex: " + re.String())
	v := int64(0)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		m := re.FindStringSubmatch(scanner.Text())
		if m == nil {
			continue
		}

		v, e = strconv.ParseInt(m[1], 0, 64)
		break
	}
	f.Close()

	return v, e
}

// ParseTopologyProperties parse for a property value in kfd topology file as string
// The format is usually one entry per line <name> <value>.  Examples in
// testdata/topology-parsing/.
func ParseTopologyPropertiesString(path string, re *regexp.Regexp) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}

	e = errors.New("Topology property not found.  Regex: " + re.String())
	v := ""
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		m := re.FindStringSubmatch(scanner.Text())
		if m == nil {
			continue
		}

		v = m[1]
		e = nil
		break
	}
	f.Close()

	return v, e
}

var fwVersionRe = regexp.MustCompile(`(\w+) feature version: (\d+), firmware version: (0x[0-9a-fA-F]+)`)

func parseDebugFSFirmwareInfo(path string) (map[string]uint32, map[string]uint32) {
	feat := make(map[string]uint32)
	fw := make(map[string]uint32)

	glog.Info("Parsing " + path)
	f, e := os.Open(path)
	if e == nil {
		scanner := bufio.NewScanner(f)
		var v int64
		for scanner.Scan() {
			m := fwVersionRe.FindStringSubmatch(scanner.Text())
			if m != nil {
				v, _ = strconv.ParseInt(m[2], 0, 32)
				feat[m[1]] = uint32(v)
				v, _ = strconv.ParseInt(m[3], 0, 32)
				fw[m[1]] = uint32(v)
			}
		}
	} else {
		glog.Error("Fail to open " + path)
	}

	return feat, fw
}

var topoDrmRenderMinorRe = regexp.MustCompile(`drm_render_minor\s(\d+)`)
var topoUniqueIdRe = regexp.MustCompile(`unique_id\s(\d+)`)

func GetNodeIdsFromTopology(topoRootParam ...string) map[int]int {
	topoRoot := "/sys/class/kfd/kfd"
	if len(topoRootParam) == 1 {
		topoRoot = topoRootParam[0]
	}

	renderNodeIds := make(map[int]int)
	var nodeFiles []string
	var err error

	if nodeFiles, err = filepath.Glob(topoRoot + "/topology/nodes/*/properties"); err != nil {
		glog.Fatalf("glob error: %s", err)
		return renderNodeIds
	}

	for _, nodeFile := range nodeFiles {
		glog.Info("Parsing " + nodeFile)
		v, e := ParseTopologyProperties(nodeFile, topoDrmRenderMinorRe)
		if e != nil {
			glog.Error(e)
			continue
		}

		if v <= 0 {
			continue
		}
		// For a certain drm_render_minor value in the properties file, we are assigning the nodeID as the <int> value from topology/nodes/<int>/properties/
		// Later we use the renderD of a certain GPU as the key to fetch its nodeID from here
		// Extract the node index (the folder name) from the file path
		nodeIndex := filepath.Base(filepath.Dir(nodeFile))

		// Convert the node index to an integer
		nodeId, err := strconv.Atoi(nodeIndex)
		if err != nil {
			glog.Errorf("Failed to convert node index %s to int: %v", nodeIndex, err)
			continue
		}

		renderNodeIds[int(v)] = nodeId
	}

	return renderNodeIds
}

func simpleHealthCheck() bool {
	entries, err := filepath.Glob("/sys/class/kfd/kfd/topology/nodes/*/properties")
	if err != nil {
		glog.Errorf("Error finding properties files: %v", err)
		return false
	}

	for _, propFile := range entries {
		f, err := os.Open(propFile)
		if err != nil {
			glog.Errorf("Error opening %s: %v", propFile, err)
			continue
		}
		defer f.Close()

		var cpuCores, gfxVersion int
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "cpu_cores_count") {
				parts := strings.Fields(line)
				if len(parts) == 2 {
					cpuCores, _ = strconv.Atoi(parts[1])
				}
			} else if strings.HasPrefix(line, "gfx_target_version") {
				parts := strings.Fields(line)
				if len(parts) == 2 {
					gfxVersion, _ = strconv.Atoi(parts[1])
				}
			}
		}

		if err := scanner.Err(); err != nil {
			glog.Warningf("Error scanning %s: %v", propFile, err)
			continue
		}

		if cpuCores == 0 && gfxVersion > 0 {
			// Found a GPU
			return true
		}
	}

	glog.Warning("No GPU nodes found via properties")
	return false
}

var topoSIMDre = regexp.MustCompile(`simd_count\s(\d+)`)

func countGPUDevFromTopology(topoRootParam ...string) int {
	topoRoot := "/sys/class/kfd/kfd"
	if len(topoRootParam) == 1 {
		topoRoot = topoRootParam[0]
	}

	count := 0
	var nodeFiles []string
	var err error
	if nodeFiles, err = filepath.Glob(topoRoot + "/topology/nodes/*/properties"); err != nil {
		glog.Fatalf("glob error: %s", err)
		return count
	}

	for _, nodeFile := range nodeFiles {
		glog.Info("Parsing " + nodeFile)
		f, e := os.Open(nodeFile)
		if e != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			m := topoSIMDre.FindStringSubmatch(scanner.Text())
			if m == nil {
				continue
			}

			if v, _ := strconv.Atoi(m[1]); v > 0 {
				count++
				break
			}
		}
		f.Close()
	}
	return count
}
