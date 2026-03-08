GOCMD      ?= go
LINT        = golangci-lint
BINARY      = build/avellcc
ENTRY       = .
LINT_GOCACHE ?= /tmp/go-cache
LINT_CACHE   ?= /tmp/golangci-lint-cache

.DEFAULT_GOAL := help

# ─── Development ──────────────────────────────────────────────

.PHONY: run
run: check-go ## Run avellcc (go run)
	$(GOCMD) run $(ENTRY)

# ─── Build ────────────────────────────────────────────────────

.PHONY: build
build: check-go ## Build Go binary to build/avellcc
	@mkdir -p build
	$(GOCMD) build -o $(BINARY) $(ENTRY)

.PHONY: build-static
build-static: check-go ## Build static stripped binary
	@mkdir -p build
	CGO_ENABLED=0 $(GOCMD) build -trimpath -ldflags='-s -w' -o $(BINARY) $(ENTRY)

# ─── Quality ──────────────────────────────────────────────────

.PHONY: test
test: check-go ## Run Go tests
	$(GOCMD) test ./...

.PHONY: test-coverage
test-coverage: check-go ## Run tests with race detection and coverage
	$(GOCMD) test -race -covermode=atomic -coverprofile=coverage.txt ./...

.PHONY: benchmark
benchmark: check-go ## Run Go benchmarks
	$(GOCMD) test -run=^$$ -bench=. -benchmem ./...

.PHONY: fmt
fmt: check-go check-lint ## Format Go code
	GOCACHE=$(LINT_GOCACHE) GOLANGCI_LINT_CACHE=$(LINT_CACHE) $(LINT) fmt

.PHONY: lint
lint: check-go check-lint ## Lint Go code
	GOCACHE=$(LINT_GOCACHE) GOLANGCI_LINT_CACHE=$(LINT_CACHE) $(LINT) run

.PHONY: tidy
tidy: check-go ## Tidy go.mod and go.sum
	$(GOCMD) mod tidy

.PHONY: vuln
vuln: check-go ## Run vulnerability scanner
	govulncheck ./...

.PHONY: ci
ci: fmt lint test build ## Run full CI pipeline

# ─── Install ─────────────────────────────────────────────────

PREFIX     ?= /usr/local
BINDIR      = $(PREFIX)/bin
UDEVDIR     = /etc/udev/rules.d
SERVICEDIR  = /etc/systemd/system

.PHONY: install
install: build-static ## Install binary, udev rules, and systemd service
	sudo install -Dm755 $(BINARY) $(BINDIR)/avellcc
	@echo "Installed avellcc to $(BINDIR)/avellcc"
	@sudo cp udev/99-avell-keyboard.rules $(UDEVDIR)/
	@sudo udevadm control --reload-rules && sudo udevadm trigger
	@echo "udev rules installed."
	@sudo cp systemd/avellcc-restore.service $(SERVICEDIR)/
	@sudo systemctl daemon-reload
	@echo "systemd service installed."
	@echo "  Enable: sudo systemctl enable avellcc-restore.service"

.PHONY: uninstall
uninstall: ## Remove binary, udev rules, and systemd service
	-sudo systemctl disable --now avellcc-restore.service 2>/dev/null
	sudo rm -f $(BINDIR)/avellcc
	sudo rm -f $(UDEVDIR)/99-avell-keyboard.rules
	sudo rm -f $(SERVICEDIR)/avellcc-restore.service
	-sudo udevadm control --reload-rules 2>/dev/null
	-sudo systemctl daemon-reload 2>/dev/null
	@echo "avellcc uninstalled."

# ─── Maintenance ──────────────────────────────────────────────

.PHONY: clean
clean: ## Remove build artifacts
	$(GOCMD) clean
	rm -rf build coverage.txt

.PHONY: help
help: ## Show available targets
	@awk '\
		/^# ─── / { printf "\n\033[1m%s\033[0m\n", substr($$0, 7) } \
		/^[a-zA-Z_-]+:.*## / { \
			target = $$0; \
			sub(/:.*/, "", target); \
			desc = $$0; \
			sub(/.*## /, "", desc); \
			printf "  \033[36m%-18s\033[0m %s\n", target, desc; \
		}' $(MAKEFILE_LIST)
	@echo

# Dependency checks (internal)

.PHONY: check-go check-lint

check-go:
	@command -v $(GOCMD) >/dev/null 2>&1 || { echo "error: go is not installed — https://golang.org/doc/install"; exit 1; }

check-lint:
	@command -v $(LINT) >/dev/null 2>&1 || { echo "error: $(LINT) is not installed — go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"; exit 1; }
