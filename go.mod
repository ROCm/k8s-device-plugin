module github.com/RadeonOpenCompute/k8s-device-plugin

go 1.16

require (
	github.com/go-logr/logr v0.3.0
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/kubevirt/device-plugin-manager v1.18.8
	golang.org/x/net v0.0.0-20201110031124-69a78807bb2b
	google.golang.org/grpc v1.35.0 // indirect
	k8s.io/api v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/kubelet v0.18.8
	sigs.k8s.io/controller-runtime v0.8.1
)
