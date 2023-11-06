ARG BUILDER_IMAGE=terway-qos-builder
ARG UBUNTU_IMAGE=docker.io/library/ubuntu:22.04@sha256:2b7412e6465c3c7fc5bb21d3e6f1917c167358449fecac8176c6e496e5c1f05f

FROM --platform=$TARGETPLATFORM ${BUILDER_IMAGE} as builder-dist

FROM --platform=$TARGETPLATFORM ${UBUNTU_IMAGE}
COPY --from=builder-dist /usr/bin/llvm-strip-15 /usr/bin/clang-15 /usr/bin/llvm-objcopy-15 /usr/local/bin/