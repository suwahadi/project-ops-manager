TAILWIND ?= tailwindcss
SQLC ?= sqlc

.PHONY: css generate fmt vet test check build run

css:
	$(TAILWIND) -i internal/web/static/css/input.css -o internal/web/static/css/app.css --minify

generate:
	$(SQLC) generate

fmt:
	gofmt -w .

vet:
	go vet ./...

test:
	go test ./...

check:
	test -z "$$(gofmt -l .)"
	go vet ./...
	$(SQLC) generate
	git diff --exit-code internal/db/sqlc
	go test ./...

build: css
	CGO_ENABLED=0 go build -trimpath -o bin/opsmanager ./cmd/opsmanager

run: css
	go run ./cmd/opsmanager serve
