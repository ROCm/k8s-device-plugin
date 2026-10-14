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
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ROCm/k8s-device-plugin/internal/pkg/allocator"
	"github.com/ROCm/k8s-device-plugin/internal/pkg/amdgpu"
	"github.com/ROCm/k8s-device-plugin/internal/pkg/exporter"
	"github.com/golang/glog"
	"github.com/kubevirt/device-plugin-manager/pkg/dpm"
	"golang.org/x/net/context"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

// Plugin is identical to DevicePluginServer interface of device plugin API.
type AMDGPUPlugin struct {
	AMDGPUs            map[string]map[string]interface{}
	Heartbeat          chan bool
	signal             chan os.Signal
	Resource           string
	devAllocator       allocator.Policy
	allocatorInitError bool
	occupancyCheck     bool
	occupancyReclaim   bool
	reclaimTimeout     time.Duration
	reclaimCooldown    time.Duration
	inspector          *kfdInspector
	quarantine         map[string]time.Time
	quarantineMu       sync.Mutex
	healthNotify       chan struct{}
	recheckInterval    time.Duration
	recheckRunning     bool
}

// quarantineRecheckInterval is how often a quarantined GPU is checked for
// cooldown expiry or released VRAM when kubelet sends no further RPCs.
const quarantineRecheckInterval = time.Second

// quarantineBeforeClear runs while expireQuarantine holds no lock.
var quarantineBeforeClear func()

type AMDGPUPluginOption func(*AMDGPUPlugin)

func NewAMDGPUPlugin(options ...AMDGPUPluginOption) *AMDGPUPlugin {
	amdGpuPlugin := &AMDGPUPlugin{
		healthNotify:    make(chan struct{}, 1),
		quarantine:      map[string]time.Time{},
		inspector:       newKFDInspector(),
		reclaimTimeout:  DefaultReclaimTimeout,
		reclaimCooldown: DefaultReclaimCooldown,
		recheckInterval: quarantineRecheckInterval,
	}
	for _, option := range options {
		option(amdGpuPlugin)
	}
	return amdGpuPlugin
}

func WithAllocator(a allocator.Policy) AMDGPUPluginOption {
	return func(p *AMDGPUPlugin) {
		p.devAllocator = a
	}
}

func WithHeartbeat(ch chan bool) AMDGPUPluginOption {
	return func(p *AMDGPUPlugin) {
		p.Heartbeat = ch
	}
}
func WithResource(res string) AMDGPUPluginOption {
	return func(p *AMDGPUPlugin) {
		p.Resource = res
	}
}

func WithOccupancyCheck(enabled bool) AMDGPUPluginOption {
	return func(p *AMDGPUPlugin) {
		p.occupancyCheck = enabled
	}
}

func WithOccupancyReclaim(enabled bool, timeout, cooldown time.Duration) AMDGPUPluginOption {
	return func(p *AMDGPUPlugin) {
		p.occupancyReclaim = enabled
		if enabled {
			p.occupancyCheck = true
		}
		if timeout > 0 {
			p.reclaimTimeout = timeout
		}
		if cooldown > 0 {
			p.reclaimCooldown = cooldown
		}
	}
}

// Start is an optional interface that could be implemented by plugin.
// If case Start is implemented, it will be executed by Manager after
// plugin instantiation and before its registration to kubelet. This
// method could be used to prepare resources before they are offered
// to Kubernetes.
func (p *AMDGPUPlugin) Start() error {
	p.signal = make(chan os.Signal, 1)
	signal.Notify(p.signal, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	err := p.devAllocator.Init(getDevices(), "")
	if err != nil {
		glog.Errorf("allocator init failed. Falling back to kubelet default allocation. Error %v", err)
		p.allocatorInitError = true
	}
	return nil
}

func getDevices() []*allocator.Device {
	devices := amdgpu.GetAMDGPUs()
	var deviceList []*allocator.Device

	for id, deviceData := range devices {
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
	return deviceList
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

// GetDevicePluginOptions returns options to be communicated with Device
// Manager
func (p *AMDGPUPlugin) GetDevicePluginOptions(ctx context.Context, e *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	if p.allocatorInitError {
		return &pluginapi.DevicePluginOptions{
			PreStartRequired: p.occupancyCheck,
		}, nil
	}
	return &pluginapi.DevicePluginOptions{
		GetPreferredAllocationAvailable: true,
		PreStartRequired:                p.occupancyCheck,
	}, nil
}

// PreStartContainer is expected to be called before each container start if indicated by plugin during registration phase.
// PreStartContainer allows kubelet to pass reinitialized devices to containers.
// PreStartContainer allows Device Plugin to run device specific operations on the Devices requested
func (p *AMDGPUPlugin) PreStartContainer(ctx context.Context, r *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	if err := p.prepareRequestedGPUs(ctx, r.DevicesIDs); err != nil {
		return nil, err
	}
	return &pluginapi.PreStartContainerResponse{}, nil
}

// ListAndWatch returns a stream of List of Devices
// Whenever a Device state change or a Device disappears, ListAndWatch
// returns the new list
func (p *AMDGPUPlugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {

	p.AMDGPUs = amdgpu.GetAMDGPUs()

	glog.Infof("Found %d AMDGPUs", len(p.AMDGPUs))

	devs := make([]*pluginapi.Device, len(p.AMDGPUs))
	var isHomogeneous bool
	isHomogeneous = amdgpu.IsHomogeneous()
	// Initialize a map to store partitionType based device list
	resourceTypeDevs := make(map[string][]*pluginapi.Device)

	if isHomogeneous {
		// limit scope for hwloc
		func() {
			i := 0
			for id, device := range p.AMDGPUs {
				dev := &pluginapi.Device{
					ID:     id,
					Health: pluginapi.Healthy,
				}
				devs[i] = dev
				i++

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
		}()
	} else {
		func() {
			for id, device := range p.AMDGPUs {
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
		}()
	}

	watched := func() []*pluginapi.Device {
		if isHomogeneous {
			return devs
		}
		list, ok := resourceTypeDevs[p.Resource]
		if !ok {
			return nil
		}
		return list
	}
	publish := func(refresh bool) {
		list := watched()
		if !isHomogeneous && list == nil {
			return
		}
		p.publishDevices(s, list, refresh)
	}
	publish(false)

loop:
	for {
		select {
		case <-p.Heartbeat:
			publish(true)
		case <-p.healthNotify:
			publish(true)

		case <-s.Context().Done():
			glog.Errorf("ListAndWatch stream disconnected: %v, exiting to trigger re-registration", s.Context().Err())
			os.Exit(1)

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
func (p *AMDGPUPlugin) GetPreferredAllocation(ctx context.Context, req *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	response := &pluginapi.PreferredAllocationResponse{}
	for _, containerReq := range req.ContainerRequests {
		available := p.availableForAllocation(containerReq.AvailableDeviceIDs, containerReq.MustIncludeDeviceIDs, int(containerReq.AllocationSize))
		allocated_ids, err := p.devAllocator.Allocate(available, containerReq.MustIncludeDeviceIDs, int(containerReq.AllocationSize))
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

// Allocate is called during container creation so that the Device
// Plugin can run device specific operations and instruct Kubelet
// of the steps to make the Device available in the container
func (p *AMDGPUPlugin) Allocate(ctx context.Context, r *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	var response pluginapi.AllocateResponse
	var car pluginapi.ContainerAllocateResponse
	var dev *pluginapi.DeviceSpec

	for _, req := range r.ContainerRequests {
		car = pluginapi.ContainerAllocateResponse{}

		if err := p.prepareRequestedGPUs(ctx, req.DevicesIDs); err != nil {
			return nil, err
		}

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

func (p *AMDGPUPlugin) publishDevices(s pluginapi.DevicePlugin_ListAndWatchServer, devs []*pluginapi.Device, refresh bool) {
	if refresh {
		health := pluginapi.Unhealthy
		if simpleHealthCheck() {
			health = pluginapi.Healthy
		}
		exporter.PopulatePerGPUDHealth(devs, health)
	}
	p.expireQuarantine(time.Now())
	p.applyQuarantine(devs)
	s.Send(&pluginapi.ListAndWatchResponse{Devices: devs})
}

// prepareRequestedGPUs logs KFD VRAM occupants and, when reclaim is enabled,
// signals them and waits until the GPU is free. A reclaim timeout quarantines
// the GPU so the next allocation can choose a different one.
func (p *AMDGPUPlugin) prepareRequestedGPUs(ctx context.Context, ids []string) error {
	if !p.occupancyCheck || len(ids) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.expireQuarantine(time.Now())
	var first error
	for _, id := range ids {
		if err := p.prepareOneGPU(ctx, id); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// kfdCounterID is the suffix of the per-process VRAM file for this GPU.
// Prefer the topology gpu_id. A gpu_id file that could not be read fails
// closed. Devices discovered on a tree that has no gpu_id file fall back
// to the topology node index.
func kfdCounterID(device map[string]interface{}) (int, bool) {
	if device == nil {
		return 0, false
	}
	if unavailable, ok := device["gpuIdUnavailable"].(bool); ok && unavailable {
		return 0, false
	}
	if id, ok := device["gpuId"].(int); ok && id > 0 {
		return id, true
	}
	id, ok := device["nodeId"].(int)
	if !ok || id < 0 {
		return 0, false
	}
	return id, true
}

func (p *AMDGPUPlugin) prepareOneGPU(ctx context.Context, id string) error {
	if p.occupancyReclaim && p.isQuarantined(id) {
		return fmt.Errorf("GPU %s is quarantined after VRAM reclaim failed", id)
	}
	device, ok := p.AMDGPUs[id]
	if !ok || device == nil {
		return fmt.Errorf("cannot check occupancy for unknown GPU %s", id)
	}
	counterID, ok := kfdCounterID(device)
	if !ok {
		return fmt.Errorf("cannot check occupancy for GPU %s: KFD gpu_id unavailable", id)
	}
	if p.inspector == nil {
		return fmt.Errorf("cannot inspect KFD process state for GPU %s", id)
	}
	err := p.inspector.prepare(ctx, id, counterID, p.occupancyReclaim, p.reclaimTimeout)
	if err != nil {
		var timeout *reclaimTimeoutError
		if errors.As(err, &timeout) {
			p.quarantineGPU(id)
		}
		return err
	}
	if p.clearQuarantine(id) {
		p.notifyHealth()
	}
	return nil
}

func (p *AMDGPUPlugin) availableForAllocation(available, mustInclude []string, size int) []string {
	p.expireQuarantine(time.Now())
	blocked := p.quarantinedSet()
	if len(blocked) == 0 {
		return available
	}
	for _, id := range mustInclude {
		if _, ok := blocked[id]; ok {
			return available
		}
	}
	filtered := make([]string, 0, len(available))
	for _, id := range available {
		if _, ok := blocked[id]; !ok {
			filtered = append(filtered, id)
		}
	}
	if len(filtered) < size {
		return available
	}
	return filtered
}

func (p *AMDGPUPlugin) quarantineGPU(id string) {
	cooldown := p.reclaimCooldown
	if cooldown <= 0 {
		cooldown = DefaultReclaimCooldown
	}
	until := time.Now().Add(cooldown)
	p.quarantineMu.Lock()
	if p.quarantine == nil {
		p.quarantine = map[string]time.Time{}
	}
	p.quarantine[id] = until
	startRecheck := !p.recheckRunning && p.recheckInterval > 0
	if startRecheck {
		p.recheckRunning = true
	}
	interval := p.recheckInterval
	p.quarantineMu.Unlock()
	glog.Errorf("GPU %s quarantined until %s because VRAM reclaim did not finish", id, until.Format(time.RFC3339))
	p.notifyHealth()
	if startRecheck {
		go p.quarantineRecheckLoop(interval)
	}
}

// quarantineRecheckLoop expires cooldowns and republishes health without
// waiting for another Allocate or PreStartContainer call. It stops once no
// GPU remains quarantined.
func (p *AMDGPUPlugin) quarantineRecheckLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		p.expireQuarantine(time.Now())
		p.quarantineMu.Lock()
		empty := len(p.quarantine) == 0
		if empty {
			p.recheckRunning = false
		}
		p.quarantineMu.Unlock()
		if empty {
			return
		}
	}
}

func (p *AMDGPUPlugin) clearQuarantine(id string) bool {
	p.quarantineMu.Lock()
	defer p.quarantineMu.Unlock()
	if _, ok := p.quarantine[id]; !ok {
		return false
	}
	delete(p.quarantine, id)
	return true
}

func (p *AMDGPUPlugin) isQuarantined(id string) bool {
	p.quarantineMu.Lock()
	defer p.quarantineMu.Unlock()
	_, ok := p.quarantine[id]
	return ok
}

func (p *AMDGPUPlugin) quarantinedSet() map[string]struct{} {
	p.quarantineMu.Lock()
	defer p.quarantineMu.Unlock()
	blocked := make(map[string]struct{}, len(p.quarantine))
	for id := range p.quarantine {
		blocked[id] = struct{}{}
	}
	return blocked
}

func (p *AMDGPUPlugin) expireQuarantine(now time.Time) {
	p.quarantineMu.Lock()
	pending := make(map[string]time.Time, len(p.quarantine))
	for id, until := range p.quarantine {
		pending[id] = until
	}
	p.quarantineMu.Unlock()
	if len(pending) == 0 {
		return
	}

	if quarantineBeforeClear != nil {
		quarantineBeforeClear()
	}

	type quarantineCandidate struct {
		id    string
		until time.Time
	}
	var clear []quarantineCandidate
	for id, until := range pending {
		if !now.Before(until) || p.gpuVRAMFree(id) {
			clear = append(clear, quarantineCandidate{id: id, until: until})
		}
	}
	if len(clear) == 0 {
		return
	}

	p.quarantineMu.Lock()
	cleared := false
	for _, item := range clear {
		current, ok := p.quarantine[item.id]
		// A reclaim that timed out again while VRAM was being probed
		// installs a later deadline. Drop only the snapshot we decided on.
		if !ok || !current.Equal(item.until) {
			continue
		}
		delete(p.quarantine, item.id)
		cleared = true
		glog.Infof("GPU %s left VRAM quarantine", item.id)
	}
	p.quarantineMu.Unlock()
	if cleared {
		p.notifyHealth()
	}
}

func (p *AMDGPUPlugin) gpuVRAMFree(id string) bool {
	if p.inspector == nil || p.AMDGPUs == nil {
		return false
	}
	device, ok := p.AMDGPUs[id]
	if !ok || device == nil {
		return false
	}
	counterID, ok := kfdCounterID(device)
	if !ok {
		return false
	}
	occupants, err := p.inspector.listOccupants(counterID)
	if err != nil {
		return false
	}
	return len(occupants) == 0
}

func (p *AMDGPUPlugin) applyQuarantine(devs []*pluginapi.Device) {
	blocked := p.quarantinedSet()
	if len(blocked) == 0 {
		return
	}
	for _, dev := range devs {
		if _, ok := blocked[dev.ID]; ok {
			dev.Health = pluginapi.Unhealthy
		}
	}
}

func (p *AMDGPUPlugin) notifyHealth() {
	if p.healthNotify == nil {
		return
	}
	select {
	case p.healthNotify <- struct{}{}:
	default:
	}
}

// Lister serves as an interface between imlementation and Manager machinery. User passes
// implementation of this interface to NewManager function. Manager will use it to obtain resource
// namespace, monitor available resources and instantate a new plugin for them.
type AMDGPULister struct {
	ResUpdateChan    chan dpm.PluginNameList
	Heartbeat        chan bool
	Signal           chan os.Signal
	OccupancyCheck   bool
	OccupancyReclaim bool
	ReclaimTimeout   time.Duration
	ReclaimCooldown  time.Duration
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
	options := []AMDGPUPluginOption{
		WithHeartbeat(l.Heartbeat),
		WithResource(resourceLastName),
		WithAllocator(allocator.NewBestEffortPolicy()),
		WithOccupancyCheck(l.OccupancyCheck),
		WithOccupancyReclaim(l.OccupancyReclaim, l.ReclaimTimeout, l.ReclaimCooldown),
	}
	return NewAMDGPUPlugin(options...)
}
