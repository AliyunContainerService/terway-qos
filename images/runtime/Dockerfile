ARG UBUNTU_IMAGE=docker.io/library/ubuntu:22.04@sha256:2b7412e6465c3c7fc5bb21d3e6f1917c167358449fecac8176c6e496e5c1f05f

FROM --platform=$TARGETPLATFORM ${UBUNTU_IMAGE}
RUN apt-get update && apt-get install -y \
    clang-15 \
    llvm-15 \
    && rm -rf /var/lib/apt/lists/*
RUN ln -s /usr/local/bin/llvm-strip-15 /usr/local/bin/llvm-strip && \
    ln -s /usr/local/bin/llvm-objcopy-15 /usr/local/bin/llvm-objcopy && \
    ln -s /usr/local/bin/clang-15 /usr/local/bin/clang