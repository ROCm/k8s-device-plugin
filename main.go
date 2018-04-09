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
package main

import (
	"flag"
	//"fmt"
	"os"

	//"github.com/golang/glog"
	"github.com/kubevirt/device-plugin-manager/pkg/dpm"
	"golang.org/x/net/context"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1alpha"
)

const (
	devID = "0x1002"
)

type Plugin struct{}

func (p *Plugin) Start() error {
	return nil
}

func (p *Plugin) Stop() error {
	return nil
}

// Monitors available amdgpu devices and notifies Kubernetes
func (p *Plugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
	devs := make([]*pluginapi.Device, 0)

	// TODO implement a more sophisticated ways to find the number of GPU available
	// TODO register multiple GPUs per node if available
	devs = append(devs, &pluginapi.Device{
		ID:     "gpu",
		Health: pluginapi.Healthy,
	})
	s.Send(&pluginapi.ListAndWatchResponse{Devices: devs})

	for {
		select {
		//TODO implement health monitor and other control mechanisms
		}
	}
	// returning a value with this function will unregister the plugin from k8s
	return nil
}

func (p *Plugin) Allocate(ctx context.Context, r *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	var response pluginapi.AllocateResponse

	// Currently, there are only 1 /dev/kfd per nodes regardless of the # of GPU available
	dev := new(pluginapi.DeviceSpec)
	dev.HostPath = "/dev/kfd"
	dev.ContainerPath = "/dev/kfd"
	dev.Permissions = "rw"
	response.Devices = append(response.Devices, dev)

	dev = new(pluginapi.DeviceSpec)
	dev.HostPath = "/dev/dri"
	dev.ContainerPath = "/dev/dri"
	dev.Permissions = "rw"
	response.Devices = append(response.Devices, dev)

	return &response, nil
}

type Lister struct {
	ResUpdateChan chan dpm.PluginNameList
}

func (l *Lister) GetResourceNamespace() string {
	return "amd.com"
}

// Monitors available resources
func (l *Lister) Discover(pluginListCh chan dpm.PluginNameList) {
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

func (l *Lister) NewPlugin(resourceLastName string) dpm.PluginInterface {
	return &Plugin{}
}

func main() {

	// this is also needed to enable glog usage in dpm
	flag.Parse()

	l := Lister{
		ResUpdateChan: make(chan dpm.PluginNameList),
	}
	manager := dpm.NewManager(&l)

	go func() {
		// /sys/class/kfd only exists if ROCm kernel/driver is installed
		var path = "/sys/class/kfd"
		if _, err := os.Stat(path); err == nil {
			l.ResUpdateChan <- []string{"gpu"}
		}
	}()
	manager.Run()

}
