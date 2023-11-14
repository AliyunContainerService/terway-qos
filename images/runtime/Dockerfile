ARG LLVM_IMAGE=registry.cn-hangzhou.aliyuncs.com/l1b0k/cilium-llvm:a8c542efc076b62ba683e7699c0013adb6955f0f-wip
ARG UBUNTU_IMAGE=docker.io/library/ubuntu:22.04@sha256:2b7412e6465c3c7fc5bb21d3e6f1917c167358449fecac8176c6e496e5c1f05f

FROM  ${LLVM_IMAGE} as llvm-dist

FROM --platform=$TARGETPLATFORM ${UBUNTU_IMAGE}
COPY --from=llvm-dist /usr/local/bin/clang /usr/local/bin/llc /bin/