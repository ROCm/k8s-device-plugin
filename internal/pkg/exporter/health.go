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
    "github.com/ROCm/k8s-device-plugin/internal/pkg/exporter/metricssvc"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    "google.golang.org/protobuf/types/known/emptypb"
    "github.com/golang/glog"
    pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
    healthSocket = "/var/lib/amd-metrics-exporter/amdgpu_device_metrics_exporter_grpc.socket"
)

// GetGPUHealth returns device id map with health state if the metrics service
// is available else returns error
func GetGPUHealth() (hMap map[string]string, err error) {
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

    resp, err := client.List(context.Background(), &emptypb.Empty{})
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
