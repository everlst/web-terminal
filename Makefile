.PHONY: test test-web test-go build image

VERSION ?= 0.1.1

test: test-web test-go

test-web:
	cd web && npm ci && npm test && npm run build

test-go:
	docker run --rm -v "$(CURDIR)":/src -w /src golang:1.25-alpine sh -c 'go test ./...'

build:
	cd web && npm ci && npm run build
	docker run --rm -v "$(CURDIR)":/src -w /src golang:1.25-alpine sh -c 'go build ./...'

image:
	docker build --build-arg VERSION=$(VERSION) -t evlst/web-terminal:$(VERSION) .
