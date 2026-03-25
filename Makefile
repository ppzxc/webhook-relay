VERSION  ?= $(shell git tag --sort=-v:refname --merged HEAD 2>/dev/null | head -1 || git describe --tags --always --dirty 2>/dev/null || echo "dev")
BINARY   := relaybox
CMD      := ./cmd/server/
DIST     := dist
LDFLAGS  := -s -w -X main.version=$(VERSION)

PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

DOCKER_IMAGE  := relaybox
DOCKER_TAG    := $(VERSION)
COMPOSE       := docker compose

.PHONY: build build-all checksums test vet clean release \
        docker-build docker-up docker-down docker-logs

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(CMD)

build-all: # NOTE: $(eval) inside $(foreach) is not safe with 'make -j'; run sequentially
	@mkdir -p $(DIST)
	$(foreach platform,$(PLATFORMS), \
		$(eval GOOS   = $(word 1,$(subst /, ,$(platform)))) \
		$(eval GOARCH = $(word 2,$(subst /, ,$(platform)))) \
		$(eval EXT    = $(if $(filter windows,$(GOOS)),.exe,)) \
		$(eval OUT    = $(DIST)/$(BINARY)-$(GOOS)-$(GOARCH)$(EXT)) \
		CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -o $(OUT) $(CMD) && \
		echo "  built $(OUT)" && \
	) true

checksums: # NOTE: uses sha256sum (Linux/CI); on macOS install coreutils or use 'shasum -a 256'
	@cd $(DIST) && sha256sum $(BINARY)-* > checksums.txt 2>/dev/null || \
		shasum -a 256 $(BINARY)-* > checksums.txt
	@echo "  checksums written to $(DIST)/checksums.txt"

test:
	go test -race ./... -timeout 60s

vet:
	go vet ./...

clean:
	rm -rf $(DIST) $(BINARY)

release: clean build-all checksums

# ── Docker ───────────────────────────────────────────────
docker-build:
	docker build --build-arg VERSION=$(VERSION) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		-t $(DOCKER_IMAGE):latest .

docker-up:
	$(COMPOSE) up -d

docker-down:
	$(COMPOSE) down

docker-logs:
	$(COMPOSE) logs -f relaybox
