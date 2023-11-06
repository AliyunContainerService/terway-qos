ARG GOLANG_IMAGE=docker.io/library/golang:1.21.3@sha256:24a09375a6216764a3eda6a25490a88ac178b5fcb9511d59d0da5ebf9e496474
ARG UBUNTU_IMAGE=docker.io/library/ubuntu:22.04@sha256:2b7412e6465c3c7fc5bb21d3e6f1917c167358449fecac8176c6e496e5c1f05f

FROM --platform=$TARGETPLATFORM ${GOLANG_IMAGE} as go-dist

FROM --platform=$TARGETPLATFORM ${UBUNTU_IMAGE}
RUN apt-get update && apt-get install -y \
    clang-15 \
    llvm-15 \
    git \
    make \
    && rm -rf /var/lib/apt/lists/*
COPY --from=go-dist /usr/local/go /usr/local/go
ENV PATH=$PATH:/usr/local/go/bin