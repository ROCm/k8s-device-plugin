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
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ROCm/k8s-device-plugin/internal/pkg/amdgpu"
	"github.com/ROCm/k8s-device-plugin/internal/pkg/hwloc"
	"github.com/ROCm/k8s-device-plugin/internal/pkg/plugin"
	"github.com/golang/glog"
	"github.com/kubevirt/device-plugin-manager/pkg/dpm"
)

var gitDescribe string

func getResourceList() []string {
	var resources []string

	// Check if the node is homogeneous
	isHomogeneous := amdgpu.IsHomogeneous()
	if isHomogeneous {
		// Homogeneous node will report only "gpu" resource
		resources = []string{"gpu"}
	} else {
		// Heterogeneous node reports resources based on partition types
		_, deviceCountMap := amdgpu.GetAMDGPUs()
		for partitionType, count := range deviceCountMap {
			if count > 0 {
				resources = append(resources, partitionType)
			}
		}
	}
	return resources
}

func main() {
	versions := [...]string{
		"AMD GPU device plugin for Kubernetes",
		fmt.Sprintf("%s version %s", os.Args[0], gitDescribe),
		fmt.Sprintf("%s", hwloc.GetVersions()),
	}

	flag.Usage = func() {
		for _, v := range versions {
			fmt.Fprintf(os.Stderr, "%s\n", v)
		}
		fmt.Fprintln(os.Stderr, "Usage:")
		flag.PrintDefaults()
	}
	var pulse int
	flag.IntVar(&pulse, "pulse", 0, "time between health check polling in seconds.  Set to 0 to disable.")
	// this is also needed to enable glog usage in dpm
	flag.Parse()

	for _, v := range versions {
		glog.Infof("%s", v)
	}

	l := plugin.AMDGPULister{
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
			resources := getResourceList()
			l.ResUpdateChan <- resources
		}
	}()
	manager.Run()

}
