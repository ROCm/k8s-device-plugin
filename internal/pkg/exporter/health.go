/**
# Copyright (c) Advanced Micro Devices, Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the \"License\");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an \"AS IS\" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

// Package health is a collection of utility to access health exporter grpc service 
// hosted by amd-metrics-exporter service
package exporter

import (
    "context"
    "fmt"
    "os"
    "strings"
    "time"
    "github.com/ROCm/k8s-device-plugin/internal/pkg/exporter/metricssvc"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    "google.golang.org/protobuf/types/known/emptypb"
    "github.com/golang/glog"
    pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
    healthSocket = "/var/lib/amd-metrics-exporter/amdgpu_device_metrics_exporter_grpc.socket"
    queryTimeout    = 5 * time.Second
)

// getGPUHealth returns device id map with health state if the metrics service
// is available else returns error
func getGPUHealth() (hMap map[string]string, err error) {
    // if the exporter service is not available done proceed
    healthSvcAddress := fmt.Sprintf("unix://%v", healthSocket)
    if _, err = os.Stat(healthSocket); err != nil {
        return
    }

    hMap = make(map[string]string)

    // the connection is short lived as the exporter can come and go
    // independently
    conn, err := grpc.Dial(healthSvcAddress,
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    if err != nil {
        glog.Errorf("Error opening client metrics svc : %v", err)
        return
    }

    // create a new gRPC echo client through the compiled stub
    client := metricssvc.NewMetricsServiceClient(conn)

    defer conn.Close()

    ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
    defer cancel()

    resp, err := client.List(ctx, &emptypb.Empty{})
    if err != nil {
        glog.Errorf("Error getting health info svc : %v", err)
        return
    }
    for _, gpu := range resp.GPUState {
        if gpu.Health == strings.ToLower(pluginapi.Healthy) {
            hMap[gpu.Device] = pluginapi.Healthy
        } else {
            hMap[gpu.Device] = pluginapi.Unhealthy
        }
    }
    return
}

// PopulatePerGPUDHealth populate the per gpu health status if available, 
// else return simple health status
func PopulatePerGPUDHealth(devs []*pluginapi.Device, defaultHealth string) {
    var hasHealthSvc = false
    hMap, err := getGPUHealth()
    if err == nil {
        hasHealthSvc = true
    }

    for i := 0; i < len(devs); i++ {
        if !hasHealthSvc {
            devs[i].Health = defaultHealth
        }else {
            // only use if we have the device id entry
            if gpuHealth, ok := hMap[devs[i].ID]; ok {
                devs[i].Health = gpuHealth
            } else {
                // revert to simpleHealthCheck if not found
                devs[i].Health = defaultHealth
            }
        }
    }
}
