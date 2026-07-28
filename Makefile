# Portable build for ews-meeting-reminders.
# Produces static binaries in OUT_DIR (default: bin/).

VERSION    ?= $(shell cat VERSION)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS ?= -s -w \
	-X ews-meeting-reminders/internal/version.Version=$(VERSION) \
	-X ews-meeting-reminders/internal/version.Commit=$(COMMIT) \
	-X ews-meeting-reminders/internal/version.BuildTime=$(BUILD_TIME)

GO          ?= go
DOCKER      ?= docker
CGO_ENABLED ?= 0
GOOS        ?= $(shell $(GO) env GOOS)
GOARCH      ?= $(shell $(GO) env GOARCH)
OUT_DIR     ?= bin
DOCKER_OUT  ?= $(OUT_DIR)
XDG_DATA_HOME   ?= $(HOME)/.local/share
XDG_CONFIG_HOME ?= $(HOME)/.config
SHARE      ?= $(XDG_DATA_HOME)/ews-meeting-reminders
CONFIG_DIR ?= $(XDG_CONFIG_HOME)/ews-meeting-reminders
UNIT_DIR   ?= $(XDG_CONFIG_HOME)/systemd/user
SERVICE    ?= ews-meeting-reminders.service

BINARIES := $(OUT_DIR)/ews-reminders $(OUT_DIR)/ews-test-notify

.PHONY: all help build clean test install docker-build ews-reminders ews-test-notify FORCE

all: build

help:
	@echo "Available targets:"
	@echo "  make build         Build binaries to $(OUT_DIR)"
	@echo "  make test          Run Go tests"
	@echo "  make install       Build and install user service"
	@echo "  make docker-build  Build binary via Docker to $(DOCKER_OUT)"
	@echo "  make clean         Remove build artifacts"

build: $(BINARIES)

ews-reminders: $(OUT_DIR)/ews-reminders
ews-test-notify: $(OUT_DIR)/ews-test-notify

$(OUT_DIR)/ews-reminders: FORCE
	@mkdir -p $(OUT_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		$(GO) build -trimpath -ldflags="$(LDFLAGS)" \
		-o $(OUT_DIR)/ews-reminders ./cmd/ews-reminders

$(OUT_DIR)/ews-test-notify: FORCE
	@mkdir -p $(OUT_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		$(GO) build -trimpath -ldflags="$(LDFLAGS)" \
		-o $(OUT_DIR)/ews-test-notify ./cmd/ews-test-notify

FORCE:

test:
	$(GO) test ./...

docker-build:
	@mkdir -p "$(DOCKER_OUT)"
	$(DOCKER) build \
		--build-arg "VERSION=$(VERSION)" \
		--build-arg "COMMIT=$(COMMIT)" \
		--build-arg "BUILD_TIME=$(BUILD_TIME)" \
		--target export \
		--output "type=local,dest=$(DOCKER_OUT)" \
		.
	@echo "OK: $(DOCKER_OUT)/ews-reminders"
	@"$(DOCKER_OUT)/ews-reminders" -version

install: build
	@mkdir -p "$(SHARE)" "$(CONFIG_DIR)" "$(UNIT_DIR)"
	@if command -v systemctl >/dev/null 2>&1; then \
		systemctl --user stop "$(SERVICE)" >/dev/null 2>&1 || true; \
	fi
	install -m 0755 "$(OUT_DIR)/ews-reminders" "$(SHARE)/ews-reminders"
	@if [ ! -f "$(CONFIG_DIR)/config.yaml" ]; then \
		install -m 0600 config.example.yaml "$(CONFIG_DIR)/config.yaml"; \
		echo "Created $(CONFIG_DIR)/config.yaml — edit credentials, then:"; \
		echo "  echo 'EWS_PASSWORD=...' > $(CONFIG_DIR)/env && chmod 600 $(CONFIG_DIR)/env"; \
	fi
	install -m 0644 systemd/ews-meeting-reminders.service "$(UNIT_DIR)/$(SERVICE)"
	systemctl --user daemon-reload
	systemctl --user enable --now "$(SERVICE)"
	@echo "OK: $(SHARE)/ews-reminders"
	@echo "logs: journalctl --user -u $(SERVICE) -f"

clean:
	rm -rf $(OUT_DIR)
	rm -f ews-reminders ews-test-notify
