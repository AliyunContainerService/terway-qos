ARG GOLANG_IMAGE=docker.io/library/golang:1.27.3@sha256:2920bb124f1a2e12a1d39f0cab756ae0068441ee1da7655dec7f5e374944095b
ARG UBUNTU_IMAGE=docker.io/library/ubuntu:22.04@sha256:2b7412e6465c3c7fc5bb21d3e6f1917c167358449fecac8176c6e496e5c1f05f

FROM ${GOLANG_IMAGE} AS go-dist

FROM ${UBUNTU_IMAGE}
RUN apt-get update && apt-get install -y \
    clang-15 \
    llvm-15 \
    git \
    make \
    && rm -rf /var/lib/apt/lists/*
COPY --from=go-dist /usr/local/go /usr/local/go
ENV PATH=$PATH:/usr/local/go/bin
