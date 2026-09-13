.PHONY: build test vet run clean
build:
	go build -o bin/dlna-recorder ./cmd/dlna-recorder
test:
	go test -race ./...
vet:
	go vet ./...
run:
	go run ./cmd/dlna-recorder -output recordings
clean:
	rm -rf bin recordings
