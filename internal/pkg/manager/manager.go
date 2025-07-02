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

// Kubernetes (k8s) device plugin manager to manage plugins for AMD Devices
package manager

import (
	"os"
	"time"

	"github.com/ROCm/k8s-device-plugin/internal/pkg/allocator"
	"github.com/ROCm/k8s-device-plugin/internal/pkg/plugin"
	"github.com/ROCm/k8s-device-plugin/internal/pkg/types"
	"github.com/kubevirt/device-plugin-manager/pkg/dpm"
)

// NewPluginManager creates a new dpm.Manager using the AMDGPULister and starts the heartbeat.
func NewPluginManager(pulse int, devImpl types.DeviceImpl) *dpm.Manager {
	lister := &amdGPULister{
		ResUpdateChan: make(chan dpm.PluginNameList),
		Heartbeat:     make(chan bool),
		DeviceImpl:    devImpl,
	}
	manager := dpm.NewManager(lister)

	if pulse > 0 {
		go func() {
			for {
				time.Sleep(time.Duration(pulse) * time.Second)
				lister.Heartbeat <- true
			}
		}()
	}

	go func() {
		if devImpl != nil {
			if resources := devImpl.GetResourceNames(); len(resources) > 0 {
				lister.ResUpdateChan <- resources
			}
		}
	}()

	return manager
}

// Lister serves as an interface between imlementation and Manager machinery. User passes
// implementation of this interface to NewManager function. Manager will use it to obtain resource
// namespace, monitor available resources and instantate a new plugin for them.
type amdGPULister struct {
	ResUpdateChan chan dpm.PluginNameList
	Heartbeat     chan bool
	Signal        chan os.Signal
	DeviceImpl    types.DeviceImpl
}

// GetResourceNamespace must return namespace (vendor ID) of implemented Lister. e.g. for
// resources in format "color.example.com/<color>" that would be "color.example.com".
func (l *amdGPULister) GetResourceNamespace() string {
	return "amd.com"
}

// Discover notifies manager with a list of currently available resources in its namespace.
// e.g. if "color.example.com/red" and "color.example.com/blue" are available in the system,
// it would pass PluginNameList{"red", "blue"} to given channel. In case list of
// resources is static, it would use the channel only once and then return. In case the list is
// dynamic, it could block and pass a new list each times resources changed. If blocking is
// used, it should check whether the channel is closed, i.e. Discover should stop.
func (l *amdGPULister) Discover(pluginListCh chan dpm.PluginNameList) {
	for {
		select {
		case newResourcesList := <-l.ResUpdateChan: // New resources found
			pluginListCh <- newResourcesList
		case <-pluginListCh: // Stop message received
			// Stop resourceUpdateCh
			return
		}
	}
}

// NewPlugin instantiates a plugin implementation. It is given the last name of the resource,
// e.g. for resource name "color.example.com/red" that would be "red". It must return valid
// implementation of a PluginInterface.
func (l *amdGPULister) NewPlugin(resourceLastName string) dpm.PluginInterface {
	options := []plugin.AMDGPUPluginOption{
		plugin.WithHeartbeat(l.Heartbeat),
		plugin.WithResource(resourceLastName),
		plugin.WithDeviceImpl(l.DeviceImpl),
		plugin.WithAllocator(allocator.NewBestEffortPolicy()),
	}
	return plugin.NewAMDGPUPlugin(options...)
}
