# =============================================================
#  compose-backup — cross-platform build
# =============================================================

BINARY  := compose-backup
SRC     := main.go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

OUT     := dist

.PHONY: all linux mac windows clean help

all: linux mac windows ## Build for all platforms

linux:   $(OUT)/$(BINARY)-linux-amd64 $(OUT)/$(BINARY)-linux-arm64     ## Linux  (amd64, arm64)
mac:     $(OUT)/$(BINARY)-darwin-amd64 $(OUT)/$(BINARY)-darwin-arm64   ## macOS  (Intel, Apple Silicon)
windows: $(OUT)/$(BINARY)-windows-amd64.exe                            ## Windows (amd64)

$(OUT):
	mkdir -p $(OUT)

$(OUT)/$(BINARY)-linux-amd64: $(SRC) | $(OUT)
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o $@ $(SRC)

$(OUT)/$(BINARY)-linux-arm64: $(SRC) | $(OUT)
	GOOS=linux   GOARCH=arm64 go build $(LDFLAGS) -o $@ $(SRC)

$(OUT)/$(BINARY)-darwin-amd64: $(SRC) | $(OUT)
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o $@ $(SRC)

$(OUT)/$(BINARY)-darwin-arm64: $(SRC) | $(OUT)
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o $@ $(SRC)

$(OUT)/$(BINARY)-windows-amd64.exe: $(SRC) | $(OUT)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $@ $(SRC)

clean: ## Remove build artifacts
	rm -rf $(OUT)

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'
