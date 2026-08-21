GO ?= go

.PHONY: all build test check fmt vet lint selfscan docs hooks clean

all: check build

build:
	$(GO) build ./...

test:
	$(GO) run ./internal/testparallel -race -workers 4 ./...

fmt:
	gofmt -l -w .

vet:
	$(GO) vet ./...

# check is what the pre-commit hook and CI run.
check: vet test
	@out=$$(gofmt -l . | grep -v /testdata/ || true); if [ -n "$$out" ]; then \
		echo "gofmt needed on:"; echo "$$out"; exit 1; fi

# Dogfood: run perfscan on itself at every level.
selfscan:
	$(GO) run . -level 3 ./...

# Run every Before/After micro-benchmark pair once (compile+run sanity).
bench:
	$(GO) test -run '^$$' -bench . -benchtime=1x ./benchmarks/

# Regenerate docs/checks/ from the registry.
docs:
	$(GO) run ./gendocs

hooks:
	git config core.hooksPath .githooks
	@echo "git hooks installed (core.hooksPath=.githooks)"

clean:
	$(GO) clean ./...
