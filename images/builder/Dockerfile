ARG GO_IMAGE=golang:1.21.1
FROM --platform=$TARGETPLATFORM ${GO_IMAGE} as go-dist

FROM --platform=$TARGETPLATFORM ubuntu:22.04
RUN apt-get update && apt-get install -y \
    clang-14 \
    llvm-14 \
    git \
    make \
    && rm -rf /var/lib/apt/lists/*
COPY --from=go-dist /usr/local/go /usr/local/go
ENV PATH=$PATH:/usr/local/go/bin
RUN go env -w GOPROXY=https://goproxy.cn,direct