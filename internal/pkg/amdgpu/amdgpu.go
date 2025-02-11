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

	"github.com/golang/glog"
)

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

// GetAMDGPUs return a map of AMD GPU on a node identified by the part of the pci address
func GetAMDGPUs() (map[string]map[string]interface{}, map[string]int) {
	if _, err := os.Stat("/sys/module/amdgpu/drivers/"); err != nil {
		glog.Warningf("amdgpu driver unavailable: %s", err)
		return make(map[string]map[string]interface{}), make(map[string]int)
	}

	//ex: /sys/module/amdgpu/drivers/pci:amdgpu/0000:19:00.0
	deviceCountMap := make(map[string]int)

	matches, _ := filepath.Glob("/sys/module/amdgpu/drivers/pci:amdgpu/[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]:*")

	devices := make(map[string]map[string]interface{})
	card, renderD, devID, realGPUDevID := 0, 128, 0, 0

	for _, path := range matches {
		computePartitionFile := filepath.Join(path, "current_compute_partition")
		memoryPartitionFile := filepath.Join(path, "current_memory_partition")

		computePartition, memoryPartition := "", ""

		// Read the compute partition
		if data, err := ioutil.ReadFile(computePartitionFile); err == nil {
			computePartition = strings.ToLower(strings.TrimSpace(string(data)))
		} else {
			glog.Warningf("Failed to read 'current_compute_partition' file at %s: %s", computePartitionFile, err)
			continue
		}

		// Read the memory partition
		if data, err := ioutil.ReadFile(memoryPartitionFile); err == nil {
			memoryPartition = strings.ToLower(strings.TrimSpace(string(data)))
		} else {
			glog.Warningf("Failed to read 'current_memory_partition' file at %s: %s", memoryPartitionFile, err)
			continue
		}
		overallPartition := computePartition + "_" + memoryPartition
		deviceCountMap[overallPartition]++

		glog.Info(path)
		devPaths, _ := filepath.Glob(path + "/drm/*")

		for _, devPath := range devPaths {
			switch name := filepath.Base(devPath); {
			case name[0:4] == "card":
				// Read the 'dev' file inside the 'card*' directory
				devFilePath := filepath.Join(devPath, "dev")
				if data, err := ioutil.ReadFile(devFilePath); err == nil {
					devIDStr := strings.TrimSpace(strings.Split(string(data), ":")[1])
					devID, err = strconv.Atoi(devIDStr)
					if err != nil {
						glog.Warningf("Failed to convert devID to int from %s: %s", devFilePath, err)
					}
				} else {
					glog.Warningf("Failed to read 'dev' file at %s: %s", devFilePath, err)
				}
				card, _ = strconv.Atoi(name[4:])
			case name[0:7] == "renderD":
				renderD, _ = strconv.Atoi(name[7:])
			}
		}
		// add devID from the dev file we read so that we can identify later which gpu should get reported under which resource type
		devices[filepath.Base(path)] = map[string]interface{}{"card": card, "renderD": renderD, "devID": devID, "computePartition": computePartition, "memoryPartition": memoryPartition}
	}

	// certain products have additional devices (such as MI300's partitions)
	//ex: /sys/devices/platform/amdgpu_xcp_30
	platformMatches, _ := filepath.Glob("/sys/devices/platform/amdgpu_xcp_*")

	// This is needed because some of the visible renderD are actually not valid
	// Their validity depends on topology information from KFD
    topoRenderNodes := renderNodeSetFromTopology()

	for _, path := range platformMatches {
		glog.Info(path)
		devPaths, _ := filepath.Glob(path + "/drm/*")

		for _, devPath := range devPaths {
			switch name := filepath.Base(devPath); {
			case name[0:4] == "card":
				card, _ = strconv.Atoi(name[4:])
			case name[0:7] == "renderD":
				renderD, _ = strconv.Atoi(name[7:])
			}
		}

		if !topoRenderNodes[renderD] {
			continue
		}
		computePartition, memoryPartition := "", ""
		for _, devPath := range devPaths {
			switch name := filepath.Base(devPath); {
			case name[0:4] == "card":
				// Read the 'dev' file inside the 'card*' directory
				devFilePath := filepath.Join(devPath, "dev")
				if data, err := ioutil.ReadFile(devFilePath); err == nil {
					devIDStr := strings.TrimSpace(strings.Split(string(data), ":")[1])
					devID, err = strconv.Atoi(devIDStr)
					if err != nil {
						glog.Warningf("Failed to convert devID to int from %s: %s", devFilePath, err)
					} else {
						// calculate the real gpu that it belongs to so that we have count of how many partitions each real gpu has currently
						realGPUDevID = ((devID-1)/8)*8 + 1

						// Set the computePartition and memoryPartition from the original GPU
						for _, device := range devices {
							if device["devID"] == realGPUDevID {
								computePartition = device["computePartition"].(string)
								memoryPartition = device["memoryPartition"].(string)
								break
							}
						}
						overallPartition := computePartition + "_" + memoryPartition
						deviceCountMap[overallPartition]++

					}
				} else {
					glog.Warningf("Failed to read 'dev' file at %s: %s", devFilePath, err)
				}
			}
		}

		devices[filepath.Base(path)] = map[string]interface{}{"card": card, "renderD": renderD, "devID": realGPUDevID, "computePartition": computePartition, "memoryPartition": memoryPartition}
	}
	glog.Infof("Device counts: %v", deviceCountMap)
	return devices, deviceCountMap
}

func IsHomogeneous() bool {
	_, deviceCountMap := GetAMDGPUs()
	var firstCount int

	if len(deviceCountMap) == 0 {
		return true
	}

	for _, count := range deviceCountMap {
		firstCount = count
		break
	}

	// Iterate through the deviceCountMap and check if all counts are the same
	for _, count := range deviceCountMap {
		if count != firstCount {
			return false
		}
	}

	return true
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

func renderNodeSetFromTopology(topoRootParam ...string) map[int]bool {
	topoRoot := "/sys/class/kfd/kfd"
	if len(topoRootParam) == 1 {
		topoRoot = topoRootParam[0]
	}

	renderNodes := make(map[int]bool)
	var nodeFiles []string
	var err error

	if nodeFiles, err = filepath.Glob(topoRoot + "/topology/nodes/*/properties"); err != nil {
		glog.Fatalf("glob error: %s", err)
		return renderNodes
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

		renderNodes[int(v)] = true
	}

	return renderNodes
}
