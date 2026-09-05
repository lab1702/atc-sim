.PHONY: run test build
run:
	go run .
test:
	go test -race ./...
build:
	mkdir -p bin
	go build -buildvcs=false -trimpath -o bin/atc-sim .
