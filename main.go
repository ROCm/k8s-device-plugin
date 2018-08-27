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
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/golang/glog"
	"github.com/kubevirt/device-plugin-manager/pkg/dpm"
	"golang.org/x/net/context"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1alpha"
)

const (
	devID = "0x1002"
)

type Plugin struct {
	Heartbeat chan bool
}

func (p *Plugin) Start() error {
	return nil
}

func (p *Plugin) Stop() error {
	return nil
}

var topoSIMDre = regexp.MustCompile(`simd_count\s(\d+)`)

func countGPUDev(topoRootParam ...string) int {
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

// Monitors available amdgpu devices and notifies Kubernetes
func (p *Plugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
	devCount := countGPUDev()

	devs := make([]*pluginapi.Device, devCount)

	for i := 0; i < devCount; i++ {
		devs[i] = &pluginapi.Device{
			ID:     fmt.Sprintf("gpu%d", i),
			Health: pluginapi.Healthy,
		}
	}
	s.Send(&pluginapi.ListAndWatchResponse{Devices: devs})

	for {
		select {
		case <-p.Heartbeat:
			var health = pluginapi.Unhealthy

			// TODO there are no per device health check currently
			// TODO all devices on a node is used together by kfd
			if simpleHealthCheck() {
				health = pluginapi.Healthy
			}

			for i := 0; i < devCount; i++ {
				devs[i].Health = health
			}
			s.Send(&pluginapi.ListAndWatchResponse{Devices: devs})
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
	Heartbeat     chan bool
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
	return &Plugin{
		Heartbeat: l.Heartbeat,
	}
}

var gitDescribe string

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "AMD GPU device plugin for Kubernetes\n")
		fmt.Fprintf(os.Stderr, "%s version %s\n", os.Args[0], gitDescribe)
		fmt.Fprintln(os.Stderr, "Usage:")
		flag.PrintDefaults()
	}
	var pulse int
	flag.IntVar(&pulse, "pulse", 0, "time between health check polling in seconds.  Set to 0 to disable.")
	// this is also needed to enable glog usage in dpm
	flag.Parse()

	glog.Infof("AMD GPU device plugin for Kubernetes")
	glog.Infof("%s version %s\n", os.Args[0], gitDescribe)

	l := Lister{
		ResUpdateChan: make(chan dpm.PluginNameList),
		Heartbeat:     make(chan bool),
	}
	manager := dpm.NewManager(&l)

	if pulse > 0 {
		go func() {
			glog.Infof("Heart beating every %d seconds", pulse)
			for {
				time.Sleep(time.Second * time.Duration(pulse))
				l.Heartbeat <- true
			}
		}()
	}

	go func() {
		// /sys/class/kfd only exists if ROCm kernel/driver is installed
		var path = "/sys/class/kfd"
		if _, err := os.Stat(path); err == nil {
			l.ResUpdateChan <- []string{"gpu"}
		}
	}()
	manager.Run()

}
