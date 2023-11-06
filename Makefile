
GO ?= go
DOCKER ?= docker

GOFLAGS ?= -ldflags "-s -w"

CLANG ?= clang-15
STRIP ?= llvm-strip-15
OBJCOPY ?= llvm-objcopy-15
CFLAGS ?= -g -O2 -target bpf -std=gnu99 -nostdinc -D__NR_CPUS__=4 -Werror -Wall -Wextra -Wshadow -Wno-address-of-packed-member -Wno-unknown-warning-option -Wno-gnu-variable-sized-type-not-at-end -Wimplicit-int-conversion -Wenum-conversion

BPF_BUILD_IMAGE ?= terway-qos-builder:latest
RUNTIME_IMAGE ?= terway-qos-runtime:latest
GO_LINT_IMAGE ?= golangci/golangci-lint:v1.54.2-alpine
DAEMON_IMAGE ?= terway-qos:latest

.PHONE: all
all: lint build

.PHONY: lint
lint:
	$(DOCKER) run --rm -it -v $(shell pwd):/go/src/qos \
	-w /go/src/qos \
	$(GO_LINT_IMAGE) golangci-lint -v run --timeout 5m

.PHONY: build
build: builder-image runtime-image generate daemon-image

.PHONY: builder-image
builder-image:
	@$(DOCKER) image inspect $(BPF_BUILD_IMAGE) >/dev/null 2>&1 || \
        (echo "Docker image $(BPF_BUILD_IMAGE) not found, building..." && \
        cd images/builder && \
         $(DOCKER) build -t $(BPF_BUILD_IMAGE) .)

.PHONY: runtime-image
runtime-image:
	@$(DOCKER) image inspect $(RUNTIME_IMAGE) >/dev/null 2>&1 || \
        (echo "Docker image $(RUNTIME_IMAGE) not found, building..." && \
        cd images/runtime && \
         $(DOCKER) build -t $(RUNTIME_IMAGE) .)

.PHONY: daemon-image
daemon-image:
	@$(DOCKER) build -t $(DAEMON_IMAGE) .

.PHONY: generate
generate:
	$(DOCKER) run --rm -it -v $(shell pwd):/go/src/qos \
	-w /go/src/qos \
	-e BPF_CLANG="$(CLANG)" \
	-e BPF_CFLAGS="$(CFLAGS)" \
	-e $BPF_STRIP="$(STRIP)" \
	$(BPF_BUILD_IMAGE) go generate ./...