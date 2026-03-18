.PHONY: all build test lint fmt check clean install run generate

# Binaries
ASCIISHADER = asciishader
CHISEL      = chisel
CHISEL_LSP  = chisel-lsp

GO = go

# ─── Build ───────────────────────────────────────────────────────────

all: build test lint

build: build-app build-chisel build-lsp build-json2chisel

build-app:
	$(GO) build -o $(ASCIISHADER) ./cmd/asciishader

build-chisel:
	$(GO) build -o $(CHISEL) ./pkg/chisel/cmd/chisel

build-lsp:
	$(GO) build -o $(CHISEL_LSP) ./pkg/chisel/cmd/chisel-lsp

build-json2chisel:
	$(GO) build -o json2chisel ./pkg/chisel/cmd/json2chisel

install:
	$(GO) install ./cmd/asciishader
	$(GO) install ./pkg/chisel/cmd/chisel
	$(GO) install ./pkg/chisel/cmd/chisel-lsp

# ─── Generate ───────────────────────────────────────────────────────

generate:
	$(GO) generate ./pkg/chisel/lang/

# ─── Test ────────────────────────────────────────────────────────────

test:
	$(GO) test ./... -timeout 30s -count=1

test-v:
	$(GO) test ./... -timeout 30s -count=1 -v

test-chisel:
	$(GO) test ./pkg/chisel/... -timeout 30s -count=1

test-fixtures:
	$(GO) test ./pkg/chisel -timeout 30s -count=1 -v -run TestFixtures

test-race:
	$(GO) test ./... -timeout 60s -count=1 -race

# ─── Lint & Format ──────────────────────────────────────────────────

lint:
	golangci-lint run ./...

fmt:
	gofmt -w -s .

fmt-check:
	@test -z "$$(gofmt -l -s .)" || (echo "gofmt needed on:" && gofmt -l -s . && exit 1)

# ─── Run ─────────────────────────────────────────────────────────────

run: build-app
	./$(ASCIISHADER)

# ─── Chisel ──────────────────────────────────────────────────────────

chisel-check: build-chisel
	@for f in shaders/*.chisel; do \
		echo -n "$$f: "; \
		./$(CHISEL) check "$$f" 2>&1 || true; \
	done

chisel-compile: build-chisel
	@for f in shaders/*.chisel; do \
		echo "=== $$f ==="; \
		./$(CHISEL) compile "$$f" 2>&1; \
		echo; \
	done

# ─── Clean ───────────────────────────────────────────────────────────

clean:
	rm -f $(ASCIISHADER) $(CHISEL) $(CHISEL_LSP)
	rm -f cpu.prof cpu_*.prof
	rm -f *.asciirec
