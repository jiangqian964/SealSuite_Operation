.PHONY: build run tidy clean

build:
	go build -o bin/sealsuite-operation ./cmd/main.go

run:
	go run ./cmd/main.go

tidy:
	go mod tidy

clean:
	rm -rf bin/
	rm -rf logs/
