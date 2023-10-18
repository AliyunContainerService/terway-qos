ARG UBUNTU_IMAGE=docker.io/library/ubuntu:20.04@sha256:b33325a00c7c27b23ae48cf17d2c654e2c30812b35e7846c006389318f6a71c2
ARG CILIUM_LLVM_IMAGE=quay.io/cilium/cilium-llvm:547db7ec9a750b8f888a506709adb41f135b952e@sha256:4d6fa0aede3556c5fb5a9c71bc6b9585475ac9b1064f516d4c45c8fb691c9d9e
ARG CILIUM_BPFTOOL_IMAGE=quay.io/cilium/cilium-bpftool:78448c1a37ff2b790d5e25c3d8b8ec3e96e6405f@sha256:99a9453a921a8de99899ef82e0822f0c03f65d97005c064e231c06247ad8597d
ARG CILIUM_IPROUTE2_IMAGE=quay.io/cilium/cilium-iproute2:3570d58349efb2d6b0342369a836998c93afd291@sha256:1abcd7a5d2117190ab2690a163ee9cd135bc9e4cf8a4df662a8f993044c79342

FROM  ${CILIUM_LLVM_IMAGE} as llvm-dist
FROM  ${CILIUM_BPFTOOL_IMAGE} as bpftool-dist
FROM  ${CILIUM_IPROUTE2_IMAGE} as iproute2-dist

FROM golang:1.21.1 as builder
ARG GOPROXY
ARG TARGETOS
ARG TARGETARCH
#ENV GOPROXY $GOPROXY
ENV GOPROXY https://goproxy.cn
WORKDIR /go/src/qos
COPY go.sum go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /go/src/qos/qos .

FROM $UBUNTU_IMAGE

COPY bpf/headers /var/lib/terway/headers
COPY bpf /var/lib/terway/src
COPY hack/init.sh /bin/init.sh
COPY --from=llvm-dist /usr/local/bin/clang /usr/local/bin/llc /bin/
COPY --from=bpftool-dist /usr/local /usr/local
COPY --from=iproute2-dist /usr/local /usr/local
COPY --from=iproute2-dist /usr/lib/libbpf* /usr/lib/
COPY --from=builder /go/src/qos/qos /usr/bin/