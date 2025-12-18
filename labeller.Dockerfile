# Copyright 2022 Advanced Micro Devices, Inc.  All rights reserved.
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
FROM docker.io/golang:1.23.6-alpine3.21
RUN apk --no-cache add git pkgconfig build-base libdrm-dev wget
RUN mkdir -p /go/src/github.com/ROCm/k8s-device-plugin
ADD . /go/src/github.com/ROCm/k8s-device-plugin
WORKDIR /go/src/github.com/ROCm/k8s-device-plugin/cmd/k8s-node-labeller
RUN go install \
    -ldflags="-X main.gitDescribe=$(git -C /go/src/github.com/ROCm/k8s-device-plugin/ describe --always --long --dirty)"
RUN wget https://gitlab.freedesktop.org/mesa/libdrm/-/raw/main/data/amdgpu.ids
RUN echo "74B9,   00, AMD Instinct MI325X VF" >> /go/src/github.com/ROCm/k8s-device-plugin/cmd/k8s-node-labeller/amdgpu.ids
RUN echo "738E,   01, AMD Instinct MI100" >> /go/src/github.com/ROCm/k8s-device-plugin/cmd/k8s-node-labeller/amdgpu.ids
RUN echo "73A2,   C0, AMD Radeon Pro W6900X" >> /go/src/github.com/ROCm/k8s-device-plugin/cmd/k8s-node-labeller/amdgpu.ids
RUN echo "73AB,   C0, AMD Radeon Pro W6800X" >> /go/src/github.com/ROCm/k8s-device-plugin/cmd/k8s-node-labeller/amdgpu.ids
RUN echo "74BC,   00, AMD Instinct MI308X HF VF" >> /go/src/github.com/ROCm/k8s-device-plugin/cmd/k8s-node-labeller/amdgpu.ids

FROM alpine:3.21.3
LABEL \
    org.opencontainers.image.source="https://github.com/ROCm/k8s-device-plugin" \
    org.opencontainers.image.authors="Kenny Ho <Kenny.Ho@amd.com>" \
    org.opencontainers.image.vendor="Advanced Micro Devices, Inc." \
    org.opencontainers.image.licenses="Apache-2.0"
RUN apk --no-cache add ca-certificates libdrm
WORKDIR /root/
COPY --from=0 /go/bin/k8s-node-labeller .
COPY --from=0 /go/src/github.com/ROCm/k8s-device-plugin/cmd/k8s-node-labeller/amdgpu.ids /usr/share/libdrm/amdgpu.ids
CMD ["./k8s-node-labeller"]
