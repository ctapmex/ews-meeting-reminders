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
CGO_ENABLED ?= 0
GOOS        ?= $(shell $(GO) env GOOS)
GOARCH      ?= $(shell $(GO) env GOARCH)
OUT_DIR     ?= bin

BINARIES := $(OUT_DIR)/ews-reminders $(OUT_DIR)/ews-test-notify

.PHONY: all build clean test ews-reminders ews-test-notify

all: build

build: $(BINARIES)

ews-reminders: $(OUT_DIR)/ews-reminders
ews-test-notify: $(OUT_DIR)/ews-test-notify

$(OUT_DIR)/ews-reminders:
	@mkdir -p $(OUT_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		$(GO) build -trimpath -ldflags="$(LDFLAGS)" \
		-o $(OUT_DIR)/ews-reminders ./cmd/ews-reminders

$(OUT_DIR)/ews-test-notify:
	@mkdir -p $(OUT_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		$(GO) build -trimpath -ldflags="$(LDFLAGS)" \
		-o $(OUT_DIR)/ews-test-notify ./cmd/ews-test-notify

test:
	$(GO) test ./...

clean:
	rm -rf $(OUT_DIR)
	rm -f ews-reminders ews-test-notify
