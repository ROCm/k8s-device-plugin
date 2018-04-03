// This package provides a framework (Device Plugin Manager, DPM) that makes implementation of
// Device Plugins https://kubernetes.io/docs/concepts/cluster-administration/device-plugins/
// easier. It provides abstraction of Plugins, thanks to it a user does not need to implement
// actual gRPC server. It also handles dynamic management of available resources and their
// respective plugins.
//
// Usage
//
// The framework contains two main interfaces which must be implemented by user. ListerInterface
// handles resource management, it notifies DPM about available resources. Plugin interface then
// represents a plugin that handles available devices of one resource.
//
// See Also
//
// Repository of this package and some plugins using it can be found on
// https://github.com/kubevirt/kubernetes-device-plugins/.
//
// Example
//
// Following code illustrates usage of Device Plugin Manager. Note that this is not complete
// working implementation of a plugin, but rather shows structure of it.
//
//  import (
//      "github.com/kubevirt/device-plugin-manager/pkg/dpm"
//      pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1alpha"
//  )
//
//  type Plugin struct{}
//
//  func (p *Plugin) Start() error {
//      // Set up resources if needed, initialize custom channels etc
//      return nil
//  }
//
//  func (p *Plugin) Stop() error {
//      // Tear down resources if needed
//      return nil
//  }
//
//  // Monitors available resource's devices and notifies Kubernetes
//  func (p *Plugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
//      devs := make([]*pluginapi.Device, 0)
//
//      // Set initial set of devices
//      for _, deviceID := range ... { // Iterate initial list of resource's devices
//          devs = append(devs, &pluginapi.Device{
//              ID: deviceID,
//              Health: pluginapi.Healthy,
//          }
//      }
//      s.Send(&pluginapi.ListAndWatchResponse{Devices: devs})
//
//      // Send new list of devices everytime it changes
//      devicesUpdateCh = ... // User implemented channel sending list of new devices everytime it changes
//      for {
//          select {
//          case newDevices<-devicesUpdateCh:
//              devs = make([]*pluginapi.Device, 0)
//              for _, deviceID := range ... { // Iterate initial list of resource's devices
//                  devs = append(devs, &pluginapi.Device{
//                      ID: deviceID,
//                      Health: pluginapi.Healthy,
//                  }
//              }
//              s.Send(&pluginapi.ListAndWatchResponse{Devices: devs})
//          case ...:
//              // Handle stop channel, could be passed from Stop
//          }
//      }
//  }
//
//  // Allocates a device requested by one of Pods
//  func (p *Plugin) Allocate(ctx context.Context, r *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
//      var response pluginapi.AllocateResponse
//      for _, nic := range r.DevicesIDs {
//          dev := new(pluginapi.DeviceSpec)
//          dev.HostPath = ...
//          dev.ContainerPath = ...
//          dev.Permissions = "r"
//          response.Devices = append(response.Devices, dev)
//      }
//      return &response, nil
//  }
//
//  type Lister struct{}
//
//  func (l *Lister) GetResourceNamespace() string {
//      return "color.example.com"
//  }
//
//  // Monitors available resources
//  func (l *Lister) Discover(pluginListCh chan dpm.ResourceLastNamesList) {
//      resourcesUpdateCh = ... // User implemented channel notifing about new resources
//      for {
//          select {
//          case newResourcesList := <-resourcesUpdateCh: // New resources found
//              pluginListCh <- dpm.ResourceLastNamesList(newResourceList)
//          case <-pluginListCh: // Stop message received
//              ... // Stop resourceUpdateCh
//              return
//          }
//      }
//  }
//
//  func (l *Lister) NewPlugin(resourceLastName string) dpm.PluginInterface {
//      return &Plugin{}
//  }
//
//  func main() {
//      manager := dpm.NewManager(Lister{})
//      manager.Run()
//  }
package dpm
