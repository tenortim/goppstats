.PHONY: build test release
build:
	go build

test:
	go test -v ./...

release:
	goreleaser release --clean
