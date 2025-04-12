# Copyright 2024 Advanced Micro Devices, Inc.  All rights reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
FROM registry.access.redhat.com/ubi9/ubi:latest as builder
USER root
RUN dnf install -y 'dnf-command(config-manager)' && \
    dnf config-manager --add-repo=https://mirror.stream.centos.org/9-stream/BaseOS/x86_64/os/ && \
    dnf config-manager --add-repo=https://mirror.stream.centos.org/9-stream/AppStream/x86_64/os/ && \
    rpm --import https://www.centos.org/keys/RPM-GPG-KEY-CentOS-Official && \
    dnf install git pkgconfig gcc gcc-c++ make glibc-devel binutils libdrm-devel hwloc-devel wget tar gzip -y && \
    dnf clean all
RUN wget https://golang.org/dl/go1.23.3.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go1.23.3.linux-amd64.tar.gz && \
    rm go1.23.3.linux-amd64.tar.gz
ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/go"
RUN mkdir -p /go/src/github.com/ROCm/k8s-device-plugin
ADD . /go/src/github.com/ROCm/k8s-device-plugin
WORKDIR /go/src/github.com/ROCm/k8s-device-plugin/cmd/k8s-device-plugin
RUN go install \
    -ldflags="-X main.gitDescribe=$(git -C /go/src/github.com/ROCm/k8s-device-plugin/ describe --always --long --dirty)"


FROM registry.access.redhat.com/ubi9/ubi-init:9.4
LABEL \
    name="amd-k8s-device-plugin" \ 
    maintainer="shrey.ajmera@amd.com,yan.sun3@amd.com,praveenkumar.shanmugam@amd.com,nitish.bhat@amd.com,sriram.ravishankar@amd.com,udaybhaskar.biluri@amd.com" \
    vendor="Advanced Micro Devices, Inc." \
    version="latest" \
    release="latest" \
    summary="The AMD K8s Device Plugin enables the registration of AMD GPUs in your Kubernetes cluster for compute workloads." \
    description="The AMD K8s Device Plugin enables the registration of AMD GPUs in your Kubernetes cluster for compute workloads. With the appropriate hardware and this plugin deployed in your Kubernetes cluster, you will be able to run jobs that require AMD GPU."
RUN mkdir -p /licenses && \
    dnf install -y ca-certificates libdrm && \
    rpm --import https://www.centos.org/keys/RPM-GPG-KEY-CentOS-Official && \
    dnf install -y 'dnf-command(config-manager)' && \
    dnf config-manager --add-repo=https://mirror.stream.centos.org/9-stream/BaseOS/x86_64/os/ && \
    dnf config-manager --add-repo=https://mirror.stream.centos.org/9-stream/AppStream/x86_64/os/ && \
    dnf install -y hwloc && \
    dnf clean all
ADD ./LICENSE /licenses/LICENSE
WORKDIR /root/
COPY --from=builder /go/bin/k8s-device-plugin .
CMD ["./k8s-device-plugin", "-logtostderr=true", "-stderrthreshold=INFO", "-v=5"]
