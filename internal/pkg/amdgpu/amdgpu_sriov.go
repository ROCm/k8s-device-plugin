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

// VFInfo defines a mapping between one VF and its associated PF
type VFInfo struct {
	PF string
	VF string
	ID string
}

// AMDGPUVFImpl implements the DeviceImpl interface for sriov vf-based workloads. It is
// responsible for discovering and managing AMD GPU Virtual Functions (VFs)
type AMDGPUVFImpl struct {
	initErr error
	// vfMap maps an IOMMU group ID to a slice of VFInfo structures. Each VFInfo
	// represents a discovered VF device and the PF device its associated with.
	vfMap map[string][]VFInfo
	// devList maps a resource name (e.g. "gpu") to a slice of pluginapi.Device.
	// For example, if resource "gpu" corresponds to one IOMMU group 218, devList["gpu"]
	// might contain a single pluginapi.Device with ID "218" and healthy status
	devList map[string][]*pluginapi.Device
}

func NewGPUVFImpl(initParams map[string]interface{}) (types.DeviceImpl, error) {
	impl := &AMDGPUVFImpl{}
	if err := impl.Init(); err != nil {
		return nil, err
	}
	return impl, nil
}

func (i *AMDGPUVFImpl) Init() error {
	if _, err := os.Stat(types.AMDGIMDriverPath); err != nil {
		i.initErr = fmt.Errorf("No amd gim driver loaded")
		return i.initErr
	}
	vfMap, err := GetVFMapping()
	if err != nil {
		i.initErr = fmt.Errorf("Failed to generate vf map: %v", err)
		return i.initErr
	}
	i.vfMap = vfMap

	i.devList = make(map[string][]*pluginapi.Device)
	resources := i.GetResourceNames()
	for _, r := range resources {
		i.devList[r] = i.convertToPluginDeviceList(r)
	}

	return nil
}

func (i *AMDGPUVFImpl) Start(ctx types.DevicePluginContext) error {
	return i.initErr
}

// GetResourceNames returns a slice of resource names
func (i *AMDGPUVFImpl) GetResourceNames() (resources []string) {

	if i.initErr != nil {
		return nil
	}

	// We assume we only serve VFs on this node
	return []string{types.DeviceTypeGPU}
}

// GetOptions returns the device plugin options supported for the resource
func (i *AMDGPUVFImpl) GetOptions(ctx types.DevicePluginContext) (*pluginapi.DevicePluginOptions, error) {

	if i.initErr != nil {
		return nil, i.initErr
	}

	return &pluginapi.DevicePluginOptions{}, nil
}

// Enumerate discovers available devices using the KFD interface.
func (i *AMDGPUVFImpl) Enumerate(ctx types.DevicePluginContext) ([]*pluginapi.Device, error) {

	if i.initErr != nil {
		return nil, i.initErr
	}

	glog.Infof("Found %d VF IOMMU Groups", len(i.vfMap))
	glog.Infof("resource: %s devlist: %+v\n", ctx.ResourceName(), i.devList)

	devs, _ := i.devList[ctx.ResourceName()]
	return devs, nil
}

func (i *AMDGPUVFImpl) convertToPluginDeviceList(resource string) []*pluginapi.Device {
	devList := make([]*pluginapi.Device, 0, len(i.vfMap))

	for iommu := range i.vfMap {
		dev := &pluginapi.Device{
			ID:     iommu,
			Health: pluginapi.Healthy,
		}
		devList = append(devList, dev)
	}
	return devList
}

// Allocate returns allocation details for container-based workloads using KFD
func (i *AMDGPUVFImpl) Allocate(ctx types.DevicePluginContext, r *pluginapi.AllocateRequest) (resp *pluginapi.AllocateResponse, err error) {

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
			vfDevices, ok := i.vfMap[id]
			if !ok {
				return nil, fmt.Errorf("device %s not found", id)
			}

			// Create a DeviceSpec that mounts the VFIO device corresponding to this VF's IOMMU group.
			// Every iommu group managed by vfio appears as a file in /dev/vfio. An entire iommu group
			// is allocated to a VM here. The iommu group should have 1 VF in it, but its not uncommon
			// to have all VFs belonging to a PF in the same iommu group, which would allocate all the
			// VFs to the same VM
			ds := &pluginapi.DeviceSpec{
				HostPath:      filepath.Join("/dev/vfio", id),
				ContainerPath: filepath.Join("/dev/vfio", id),
				Permissions:   "mrw",
			}
			deviceSpecs = append(deviceSpecs, ds)
			// /dev/vfio/vfio is mounted in the VM space for every Allocate request
			deviceSpecs = append(deviceSpecs, &pluginapi.DeviceSpec{
				HostPath:      filepath.Join("/dev/vfio", "vfio"),
				ContainerPath: filepath.Join("/dev/vfio", "vfio"),
				Permissions:   "mrw",
			})

			vfList := []string{}
			for _, vfInfo := range vfDevices {
				vfList = append(vfList, vfInfo.VF)
			}
			envName := fmt.Sprintf("%s_%s", types.PCIGpuPrefix, strings.ToUpper(ctx.ResourceName()))
			// Pass along additional information, such as the VF's PCI address
			envs[envName] = strings.Join(vfList, ",")
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
func (i *AMDGPUVFImpl) GetPreferredAllocation(ctx types.DevicePluginContext, req *pluginapi.PreferredAllocationRequest) (resp *pluginapi.PreferredAllocationResponse, err error) {

	if i.initErr != nil {
		return nil, i.initErr
	}

	return &pluginapi.PreferredAllocationResponse{}, nil
}

// UpdateHealth returns a health status for the devices of a given resource
func (i *AMDGPUVFImpl) UpdateHealth(ctx types.DevicePluginContext) (devices []*pluginapi.Device, err error) {

	if i.initErr != nil {
		return nil, i.initErr
	}

	var health = pluginapi.Unhealthy

	if checkDriver(types.AMDGIMDriverPath) {
		health = pluginapi.Healthy
	}

	devs, _ := i.devList[ctx.ResourceName()]

	for _, dev := range devs {
		dev.Health = health
	}

	return devs, nil
}

// GetVFMapping scans the system's PCI devices to discover AMD devices that are PFs
// bound to the gim driver and that expose SR-IOV VFs. For each eligible VF, it resolves
// the PCI address and the associated iommu group ID. It returns a vfMap with the IOMMU
// group ID as the key and a list of all VF PCI Addresses (with the PFs they are associated
// with) as the value
// Example: Consider a PF at PCI address "0000:c0:00.0" (bound to gim) that creates 2 VFs
// 0000:c0:02.0 (iommu group ID 218) and 0000:c0:02.1 (iommu group id 230). The function
// would return:
//
//	vfMap: {
//	          "218" : []VFInfo{{VF:"0000:c0:02.0", PF:"0000:c0:00.0"}}
//	          "230" : []VFInfo{{VF:"0000:c0:02.1", PF:"0000:c0:00.0"}}
//	       }
func GetVFMapping() (map[string][]VFInfo, error) {
	vfMap := make(map[string][]VFInfo)

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

		// Check if this PF is managing VFs by verifying its driver
		driverLink := filepath.Join(pciPath, "driver")
		driver, err := os.Readlink(driverLink)
		if err != nil {
			continue
		}

		// Only proceed if the driver is "gim", which indicates a PF that manages VFs
		if filepath.Base(driver) != types.AMDGIMDriverName {
			continue
		}

		// Look for SR-IOV VFs; these appear as symlinks named "virtfn*" in the PF
		// PCI Device path. Multiple VFs appear as virtfn0, virtfn1, ...
		vfPattern := filepath.Join(pciPath, "virtfn*")
		vfPaths, err := filepath.Glob(vfPattern)
		if err != nil || len(vfPaths) == 0 {
			continue
		}

		// For each VF, determine its PCI address and IOMMU group
		for _, vfPath := range vfPaths {
			// Resolve the VF symlink.
			vfTarget, err := os.Readlink(vfPath)
			if err != nil {
				continue
			}
			vfPciAddr := filepath.Base(vfTarget)

			vfFullPath := filepath.Join(types.PCIDevicePath, vfPciAddr)
			iommuGroupLink := filepath.Join(vfFullPath, "iommu_group")
			iommuGroupPath, err := os.Readlink(iommuGroupLink)
			if err != nil {
				continue
			}
			// iommu group ID
			iommuGroup := filepath.Base(iommuGroupPath)

			// extract the device ID
			deviceIDFile := filepath.Join(vfFullPath, "device")
			deviceIDBytes, err := ioutil.ReadFile(deviceIDFile)
			if err != nil {
				continue
			}
			deviceID := strings.TrimSpace(string(deviceIDBytes))
			vfInfo := VFInfo{
				PF: entry.Name(),
				VF: vfPciAddr,
				ID: deviceID,
			}
			vfMap[iommuGroup] = append(vfMap[iommuGroup], vfInfo)
			glog.Infof("Mapping IOMMU group %s: PF %s -> VF %s", iommuGroup, entry.Name(), vfPciAddr)
		}
	}
	return vfMap, nil
}

func GetGIMVersions() (string, string, error) {
	driverVersionFile := filepath.Join(types.AMDGIMModulePath, "version")
	driverSrcVersionFile := filepath.Join(types.AMDGIMModulePath, "srcversion")

	driverVersionBytes, err := ioutil.ReadFile(driverVersionFile)
	if err != nil {
		return "", "", err
	}
	driverSrcVersionBytes, err := ioutil.ReadFile(driverSrcVersionFile)
	if err != nil {
		return "", "", err
	}

	driverVersion := strings.TrimSpace(string(driverVersionBytes))
	if idx := strings.Index(driverVersion, "+"); idx != -1 {
		driverVersion = driverVersion[:idx]
	}
	return driverVersion, strings.TrimSpace(string(driverSrcVersionBytes)), nil
}
