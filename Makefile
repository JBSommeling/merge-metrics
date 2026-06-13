.PHONY: build test lint clean

build:
	go build -o bin/mergemetrics ./cmd/mergemetrics

test:
	go test ./...

lint:
	go vet ./...

clean:
	rm -rf bin/
