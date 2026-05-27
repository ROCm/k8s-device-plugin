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

package plugin

import (
	"fmt"
	"strings"

	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const sliceSeparator = "-slice-"

// buildVirtualDevices expands a list of physical device IDs into
// replicas virtual device IDs per physical device.
// Each virtual device ID follows the format "<physical-id>-slice-<n>".
// All virtual devices are initially marked as Healthy.
func buildVirtualDevices(physicalIDs []string, replicas int) []*pluginapi.Device {
	devs := make([]*pluginapi.Device, 0, len(physicalIDs)*replicas)
	for _, pid := range physicalIDs {
		for i := 0; i < replicas; i++ {
			dev := &pluginapi.Device{
				ID:     fmt.Sprintf("%s%s%d", pid, sliceSeparator, i),
				Health: pluginapi.Healthy,
			}
			devs = append(devs, dev)
		}
	}
	return devs
}

// resolvePhysicalID extracts the physical device ID from a virtual one.
// "0000:03:00.0-slice-2" → "0000:03:00.0"
// Returns the input unchanged if it contains no "-slice-" suffix,
// so the function is safe to call on physical IDs directly.
func resolvePhysicalID(virtualID string) string {
	idx := strings.LastIndex(virtualID, sliceSeparator)
	if idx < 0 {
		return virtualID
	}
	return virtualID[:idx]
}
