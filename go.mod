module github.com/RadeonOpenCompute/k8s-device-plugin

go 1.15

require (
	github.com/go-logr/logr v0.1.0
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/kubevirt/device-plugin-manager v1.10.0
	golang.org/x/net v0.0.0-20190311183353-d8887717615a
	google.golang.org/grpc v1.35.0 // indirect
	k8s.io/api v0.0.0-20190409021203-6e4e0e4f393b
	k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d
	k8s.io/kubernetes v1.13.1
	sigs.k8s.io/controller-runtime v0.2.0
)
