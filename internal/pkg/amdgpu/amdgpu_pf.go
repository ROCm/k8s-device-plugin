/**
 * Copyright 2025 Advanced Micro Devices, Inc.  All rights reserved.
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

package amdgpu

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/ROCm/k8s-device-plugin/internal/pkg/types"
	"github.com/golang/glog"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

// PFInfo holds the PF metadata
type PFInfo struct {
	PF string
	ID string
}

// AMDGPUPFImpl implements the DeviceImpl interface for pf passthrough-based workloads. It is
// responsible for discovering and managing AMD GPU Passthrough Physical Functions (PFs)
type AMDGPUPFImpl struct {
	initErr error
	// pfMap maps an IOMMU group ID to a slice of PF metadata
	pfMap map[string][]PFInfo
	// devList maps a resource name (e.g. "gpu") to a slice of pluginapi.Device.
	// For example, if resource "gpu" corresponds to one IOMMU group 218, devList["gpu"]
	// might contain a single pluginapi.Device with ID "218" and healthy status
	devList map[string][]*pluginapi.Device
	// resourceNamingStrategy controls how resources are named for Kubernetes
	resourceNamingStrategy string
}

func NewGPUPFImpl(initParams map[string]interface{}) (types.DeviceImpl, error) {
	impl := &AMDGPUPFImpl{}

	// Extract resource naming strategy from init params
	if strategy, ok := initParams[types.CmdLineResNamingStrategy]; ok {
		impl.resourceNamingStrategy = strategy.(string)
	} else {
		impl.resourceNamingStrategy = types.ResourceNamingStrategySingle // Default to single strategy
	}

	if err := impl.Init(); err != nil {
		return nil, err
	}
	return impl, nil
}

func (i *AMDGPUPFImpl) Init() error {
	if _, err := os.Stat(types.VFIODriverPath); err != nil {
		i.initErr = fmt.Errorf("No amd gim driver loaded")
		return i.initErr
	}
	pfMap, err := GetPFMapping()
	if err != nil {
		i.initErr = fmt.Errorf("Failed to generate vf map: %v", err)
		return i.initErr
	}
	i.pfMap = pfMap

	i.devList = make(map[string][]*pluginapi.Device)
	resources := i.GetResourceNames()
	for _, r := range resources {
		i.devList[r] = i.convertToPluginDeviceList(r)
	}

	return nil
}

func (i *AMDGPUPFImpl) Start(ctx types.DevicePluginContext) error {
	return i.initErr
}

// GetResourceNames returns a slice of resource names
func (i *AMDGPUPFImpl) GetResourceNames() (resources []string) {

	if i.initErr != nil {
		return nil
	}

	// For PF passthrough, return appropriate resource names based on strategy
	if i.resourceNamingStrategy == types.ResourceNamingStrategyMixed {
		return []string{types.DeviceTypeGPUPF} // In mixed mode, PF resources use "gpu_pf"
	}

	// In single mode, all resources use "gpu"
	return []string{types.DeviceTypeGPU}
}

// GetOptions returns the device plugin options supported for the resource
func (i *AMDGPUPFImpl) GetOptions(ctx types.DevicePluginContext) (*pluginapi.DevicePluginOptions, error) {

	if i.initErr != nil {
		return nil, i.initErr
	}

	return &pluginapi.DevicePluginOptions{}, nil
}

// Enumerate discovers available devices using the KFD interface.
func (i *AMDGPUPFImpl) Enumerate(ctx types.DevicePluginContext) ([]*pluginapi.Device, error) {

	if i.initErr != nil {
		return nil, i.initErr
	}

	glog.Infof("Found %d PF IOMMU Groups", len(i.pfMap))
	glog.Infof("resource: %s devlist: %+v\n", ctx.ResourceName(), i.devList)

	devs, _ := i.devList[ctx.ResourceName()]
	return devs, nil
}

func (i *AMDGPUPFImpl) convertToPluginDeviceList(resource string) []*pluginapi.Device {
	devList := make([]*pluginapi.Device, 0, len(i.pfMap))

	for iommu := range i.pfMap {
		dev := &pluginapi.Device{
			ID:     iommu,
			Health: pluginapi.Healthy,
		}
		devList = append(devList, dev)
	}
	return devList
}

// Allocate returns allocation details for container-based workloads using KFD
func (i *AMDGPUPFImpl) Allocate(ctx types.DevicePluginContext, r *pluginapi.AllocateRequest) (resp *pluginapi.AllocateResponse, err error) {

	if i.initErr != nil {
		return nil, i.initErr
	}

	responses := &pluginapi.AllocateResponse{}

	// Process each container's request
	for _, containerReq := range r.ContainerRequests {
		var deviceSpecs []*pluginapi.DeviceSpec
		envs := map[string]string{}

		for _, id := range containerReq.DevicesIDs {
			// Look up the device info (iommu group id)
			pfDevices, ok := i.pfMap[id]
			if !ok {
				return nil, fmt.Errorf("device %s not found", id)
			}

			// Create a DeviceSpec that mounts the VFIO device corresponding to this PF's IOMMU group
			// Every iommu group managed by vfio appears as a file in /dev/vfio. An entire iommu group
			// is allocated to a VM here
			ds := &pluginapi.DeviceSpec{
				HostPath:      filepath.Join("/dev/vfio", id),
				ContainerPath: filepath.Join("/dev/vfio", id),
				Permissions:   "mrw",
			}
			deviceSpecs = append(deviceSpecs, ds)
			deviceSpecs = append(deviceSpecs, &pluginapi.DeviceSpec{
				HostPath:      filepath.Join("/dev/vfio", "vfio"),
				ContainerPath: filepath.Join("/dev/vfio", "vfio"),
				Permissions:   "mrw",
			})
			pfList := []string{}
			for _, pfInfo := range pfDevices {
				pfList = append(pfList, pfInfo.PF)
			}
			envName := fmt.Sprintf("%s_%s", types.PCIGpuPrefix, strings.ToUpper(ctx.ResourceName()))
			// Pass along additional information, such as the PF's PCI address
			envs[envName] = strings.Join(pfList, ",")
		}

		containerResp := &pluginapi.ContainerAllocateResponse{
			Devices: deviceSpecs,
			Envs:    envs,
		}
		responses.ContainerResponses = append(responses.ContainerResponses, containerResp)
	}

	return responses, nil
}

// GetPreferredAllocation returns the preferred allocation response for a given resource and request
func (i *AMDGPUPFImpl) GetPreferredAllocation(ctx types.DevicePluginContext, req *pluginapi.PreferredAllocationRequest) (resp *pluginapi.PreferredAllocationResponse, err error) {

	if i.initErr != nil {
		return nil, i.initErr
	}

	return &pluginapi.PreferredAllocationResponse{}, nil
}

// UpdateHealth returns a health status for the devices of a given resource
func (i *AMDGPUPFImpl) UpdateHealth(ctx types.DevicePluginContext) (devices []*pluginapi.Device, err error) {

	if i.initErr != nil {
		return nil, i.initErr
	}

	var health = pluginapi.Unhealthy

	if checkDriver(types.VFIODriverPath) {
		health = pluginapi.Healthy
	}

	devs, _ := i.devList[ctx.ResourceName()]

	for _, dev := range devs {
		dev.Health = health
	}

	return devs, nil
}

// GetPFMapping scans the system's PCI devices to discover AMD devices that are PFs
// bound to the vfio driver that are meant to be passthrough to the VMs. For each
// eligible PF, it resolves the PCI address and the associated iommu group ID. It
// returns a pfMap with the IOMMU group ID as the key and a list of all PF PCI Addresses
// as the value
// Example: Consider 3 PFs at PCI addresses "0000:c0:00.0", "0000:d0:00.0" and "0000:e0:00.0"
// (bound to vfio) with iommu group IDs 218, 218 and 230 respectively.
// The function would return:
//
//	pfMap: {
//	          "218" : []string{"0000:c0:00.0", "0000:d0:00.0"}
//	          "230" : []string{"0000:e0:00.0"}
//	       }
func GetPFMapping() (map[string][]PFInfo, error) {
	pfMap := make(map[string][]PFInfo)

	// List all PCI devices
	entries, err := ioutil.ReadDir(types.PCIDevicePath)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %v", types.PCIDevicePath, err)
	}

	// Loop over each PCI device
	for _, entry := range entries {
		pciPath := filepath.Join(types.PCIDevicePath, entry.Name())
		vendorFile := filepath.Join(pciPath, "vendor")
		vendorBytes, err := ioutil.ReadFile(vendorFile)
		if err != nil {
			continue
		}

		vendor := strings.TrimSpace(string(vendorBytes))
		// Only consider AMD devices
		if vendor != types.AMDVendorID {
			continue
		}

		// Check if this PF is managed by vfio
		driverLink := filepath.Join(pciPath, "driver")
		driver, err := os.Readlink(driverLink)
		if err != nil {
			continue
		}

		// Only proceed if the driver is "vfio-pci". This indicates that a PF
		// is configured in the passthrough mode
		if filepath.Base(driver) != types.VFIODriverName {
			continue
		}

		iommuGroupLink := filepath.Join(pciPath, "iommu_group")
		iommuGroupPath, err := os.Readlink(iommuGroupLink)
		if err != nil {
			continue
		}
		// iommu group ID
		iommuGroup := filepath.Base(iommuGroupPath)

		// extract the device ID
		deviceIDFile := filepath.Join(pciPath, "device")
		deviceIDBytes, err := ioutil.ReadFile(deviceIDFile)
		if err != nil {
			continue
		}
		deviceID := strings.TrimSpace(string(deviceIDBytes))
		pfInfo := PFInfo{
			PF: entry.Name(),
			ID: deviceID,
		}
		pfMap[iommuGroup] = append(pfMap[iommuGroup], pfInfo)

		glog.Infof("PF: %s IOMMU group: %s", entry.Name(), iommuGroup)
	}
	return pfMap, nil
}
