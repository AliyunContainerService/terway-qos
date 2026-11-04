ARG GOLANG_IMAGE=docker.io/library/golang:1.27.3@sha256:2920bb124f1a2e12a1d39f0cab756ae0068441ee1da7655dec7f5e374944095b
ARG UBUNTU_IMAGE=docker.io/library/ubuntu:22.04@sha256:2b7412e6465c3c7fc5bb21d3e6f1917c167358449fecac8176c6e496e5c1f05f
ARG CILIUM_BPFTOOL_IMAGE=quay.io/cilium/cilium-bpftool:d3093f6aeefef8270306011109be623a7e80ad1b@sha256:2c28c64195dee20ab596d70a59a4597a11058333c6b35a99da32c339dcd7df56
ARG RUNTIME_IMAGE=terway-qos-runtime

FROM ${CILIUM_BPFTOOL_IMAGE} AS bpftool-dist

FROM ${GOLANG_IMAGE} AS builder
ARG GOPROXY=https://proxy.golang.org,direct
ARG TARGETOS
ARG TARGETARCH
ENV GOPROXY=${GOPROXY}
WORKDIR /go/src/qos
COPY go.sum go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build \
    -ldflags \
    "-s -w -X \"github.com/AliyunContainerService/terway-qos/pkg/version.gitCommit=`git rev-parse HEAD 2>/dev/null`\" \
    -X \"github.com/AliyunContainerService/terway-qos/pkg/version.buildDate=`date -u +'%Y-%m-%dT%H:%M:%SZ'`\" \
    -X \"github.com/AliyunContainerService/terway-qos/pkg/version.gitVersion=`git describe --tags --match='v*' --abbrev=14 2>/dev/null`\"" -o /go/src/qos/qos .

FROM ${RUNTIME_IMAGE}

COPY bpf/headers /var/lib/terway/headers
COPY bpf /var/lib/terway/src
COPY hack/init.sh /bin/init.sh
COPY --from=bpftool-dist /usr/local /usr/local
COPY --from=builder /go/src/qos/qos /usr/bin/
