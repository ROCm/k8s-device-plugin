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
	"github.com/ROCm/k8s-device-plugin/internal/pkg/naming"
	"github.com/ROCm/k8s-device-plugin/internal/pkg/plugin"
	"github.com/golang/glog"
	"github.com/kubevirt/device-plugin-manager/pkg/dpm"
)

var gitDescribe string

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
	var resourceNamingStrategy string
	flag.IntVar(&pulse, "pulse", 0, "time between health check polling in seconds.  Set to 0 to disable.")
	flag.StringVar(&resourceNamingStrategy, "resource_naming_strategy", "single", "Resource strategy to be used: single, mixed or product")
	// this is also needed to enable glog usage in dpm
	flag.Parse()
	strategy, err := naming.ParseStrategy(resourceNamingStrategy)
	if err != nil {
		glog.Errorf("%v", err)
		os.Exit(1)
	}

	for _, v := range versions {
		glog.Infof("%s", v)
	}

	var products naming.ProductIndex
	if strategy == naming.StrategyProduct {
		if products, err = naming.LoadProductIndex(naming.IDsPath); err != nil {
			glog.Errorf("%v: the product strategy cannot name resources without it", err)
			os.Exit(1)
		}
		glog.Infof("Loaded %d GPU products from %s", len(products), naming.IDsPath)
	}

	l := plugin.AMDGPULister{
		ResUpdateChan: make(chan dpm.PluginNameList),
		Heartbeat:     make(chan bool),
		Strategy:      strategy,
		Products:      products,
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
			resources, err := naming.ResourceList(amdgpu.GetAMDGPUs(), strategy, products)
			if err != nil {
				glog.Errorf("Error occured: %v", err)
				os.Exit(1)
			}
			if len(resources) > 0 {
				l.ResUpdateChan <- resources
			}
		}
	}()
	manager.Run()

}
