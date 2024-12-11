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

// Kubernetes (k8s) device plugin to enable registration of AMD GPU to a container cluster
package plugin

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"

	"github.com/ROCm/k8s-device-plugin/internal/pkg/amdgpu"
	"github.com/ROCm/k8s-device-plugin/internal/pkg/hwloc"
	"github.com/golang/glog"
	"github.com/kubevirt/device-plugin-manager/pkg/dpm"
	"golang.org/x/net/context"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	"github.com/ROCm/k8s-device-plugin/internal/pkg/exporter"
)

// Plugin is identical to DevicePluginServer interface of device plugin API.
type AMDGPUPlugin struct {
	AMDGPUs   map[string]map[string]int
	Heartbeat chan bool
	signal    chan os.Signal
}

// Start is an optional interface that could be implemented by plugin.
// If case Start is implemented, it will be executed by Manager after
// plugin instantiation and before its registration to kubelet. This
// method could be used to prepare resources before they are offered
// to Kubernetes.
func (p *AMDGPUPlugin) Start() error {
	p.signal =  make(chan os.Signal, 1)
	signal.Notify(p.signal, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	return nil
}

// Stop is an optional interface that could be implemented by plugin.
// If case Stop is implemented, it will be executed by Manager after the
// plugin is unregistered from kubelet. This method could be used to tear
// down resources.
func (p *AMDGPUPlugin) Stop() error {
	return nil
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

func simpleHealthCheck() bool {
	var kfd *os.File
	var err error
	if kfd, err = os.Open("/dev/kfd"); err != nil {
		glog.Error("Error opening /dev/kfd")
		return false
	}
	kfd.Close()
	return true
}

// GetDevicePluginOptions returns options to be communicated with Device
// Manager
func (p *AMDGPUPlugin) GetDevicePluginOptions(ctx context.Context, e *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

// PreStartContainer is expected to be called before each container start if indicated by plugin during registration phase.
// PreStartContainer allows kubelet to pass reinitialized devices to containers.
// PreStartContainer allows Device Plugin to run device specific operations on the Devices requested
func (p *AMDGPUPlugin) PreStartContainer(ctx context.Context, r *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

// ListAndWatch returns a stream of List of Devices
// Whenever a Device state change or a Device disappears, ListAndWatch
// returns the new list
func (p *AMDGPUPlugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
	p.AMDGPUs = amdgpu.GetAMDGPUs()

	glog.Infof("Found %d AMDGPUs", len(p.AMDGPUs))

	devs := make([]*pluginapi.Device, len(p.AMDGPUs))

	// limit scope for hwloc
	func() {
		var hw hwloc.Hwloc
		hw.Init()
		defer hw.Destroy()

		i := 0
		for id := range p.AMDGPUs {
			dev := &pluginapi.Device{
				ID:     id,
				Health: pluginapi.Healthy,
			}
			devs[i] = dev
			i++

			numas, err := hw.GetNUMANodes(id)
			glog.Infof("Watching GPU with bus ID: %s NUMA Node: %+v", id, numas)
			if err != nil {
				glog.Error(err)
				continue
			}

			if len(numas) == 0 {
				glog.Errorf("No NUMA for GPU ID: %s", id)
				continue
			}

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
	}()

	s.Send(&pluginapi.ListAndWatchResponse{Devices: devs})

	loop:
	for {
		select {
		case <-p.Heartbeat:
			var health = pluginapi.Unhealthy
			var hasHealthSvc = false

			if simpleHealthCheck() {
				health = pluginapi.Healthy
			}

			// per device health check
			hMap, err := exporter.GetGPUHealth()
			if err == nil {
				hasHealthSvc = true
			}

			for i := 0; i < len(p.AMDGPUs); i++ {
				if !hasHealthSvc {
					devs[i].Health = health
				}else {
					// only use if we have the device id entry
					if gpuHealth, ok := hMap[devs[i].ID]; ok {
						devs[i].Health = gpuHealth
					} else {
						// revert to simpleHealthCheck if not found
						devs[i].Health = health
					}
				}
			}
			s.Send(&pluginapi.ListAndWatchResponse{Devices: devs})
		case <-p.signal:
			glog.Infof("Received signal, exiting")
			break loop
		}
	}
	// returning a value with this function will unregister the plugin from k8s

	return nil
}

// GetPreferredAllocation returns a preferred set of devices to allocate
// from a list of available ones. The resulting preferred allocation is not
// guaranteed to be the allocation ultimately performed by the
// devicemanager. It is only designed to help the devicemanager make a more
// informed allocation decision when possible.
func (p *AMDGPUPlugin) GetPreferredAllocation(context.Context, *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	return &pluginapi.PreferredAllocationResponse{}, nil
}

// Allocate is called during container creation so that the Device
// Plugin can run device specific operations and instruct Kubelet
// of the steps to make the Device available in the container
func (p *AMDGPUPlugin) Allocate(ctx context.Context, r *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
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

			for k, v := range p.AMDGPUs[id] {
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

// Lister serves as an interface between imlementation and Manager machinery. User passes
// implementation of this interface to NewManager function. Manager will use it to obtain resource
// namespace, monitor available resources and instantate a new plugin for them.
type AMDGPULister struct {
	ResUpdateChan chan dpm.PluginNameList
	Heartbeat     chan bool
	Signal        chan os.Signal
}

// GetResourceNamespace must return namespace (vendor ID) of implemented Lister. e.g. for
// resources in format "color.example.com/<color>" that would be "color.example.com".
func (l *AMDGPULister) GetResourceNamespace() string {
	return "amd.com"
}

// Discover notifies manager with a list of currently available resources in its namespace.
// e.g. if "color.example.com/red" and "color.example.com/blue" are available in the system,
// it would pass PluginNameList{"red", "blue"} to given channel. In case list of
// resources is static, it would use the channel only once and then return. In case the list is
// dynamic, it could block and pass a new list each times resources changed. If blocking is
// used, it should check whether the channel is closed, i.e. Discover should stop.
func (l *AMDGPULister) Discover(pluginListCh chan dpm.PluginNameList) {
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
func (l *AMDGPULister) NewPlugin(resourceLastName string) dpm.PluginInterface {
	return &AMDGPUPlugin{
		Heartbeat: l.Heartbeat,
	}
}
