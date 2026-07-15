.PHONY: build test vet fmt clean translation-progress

GO ?= go

build:
	$(GO) build ./...

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

clean:
	$(GO) clean -cache

translation-progress:
	$(GO) run ./cmd/translation-progress > translation-progress.txt
	@echo "Translation progress written to translation-progress.txt"
