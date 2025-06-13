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

package types

import (
	"github.com/ROCm/k8s-device-plugin/internal/pkg/allocator"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

// DeviceImpl defines our device implementation interface
type DeviceImpl interface {

	// Start is called after plugin init and before registration with kubelet
	Start(ctx DevicePluginContext) error

	// GetResourceNames returns a slice of resource names
	GetResourceNames() []string

	// GetOptions returns the device plugin options supported for the resource
	GetOptions(ctx DevicePluginContext) (opt *pluginapi.DevicePluginOptions, err error)

	// Enumerate returns the list of devices for the given resource
	Enumerate(ctx DevicePluginContext) (devices []*pluginapi.Device, err error)

	// Allocate returns the allocation details for a given resource and request
	Allocate(ctx DevicePluginContext, req *pluginapi.AllocateRequest) (resp *pluginapi.AllocateResponse, err error)

	// GetPreferredAllocation returns the preferred allocation response for a given resource and request
	GetPreferredAllocation(ctx DevicePluginContext, req *pluginapi.PreferredAllocationRequest) (resp *pluginapi.PreferredAllocationResponse, err error)

	// UpdateHealth returns a health status for the devices of a given resource
	UpdateHealth(ctx DevicePluginContext) (devices []*pluginapi.Device, err error)
}

type DevicePluginContext interface {
	ResourceName() string

	SetAllocatorError(err bool)

	GetAllocator() allocator.Policy
	GetAllocatorError() bool
}
